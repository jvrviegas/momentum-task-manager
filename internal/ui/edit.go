package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

// EditField identifies one of the eight supported editable fields.
type EditField int

const (
	FieldDescription EditField = iota
	FieldProject
	FieldPriority
	FieldDue
	FieldScheduled
	FieldTags
	FieldEstimate
	FieldRecurrence
)

const editFieldCount = 8

var editFieldNames = [...]string{"Description", "Project", "Priority", "Due", "Scheduled", "Tags", "Estimate", "Recurrence"}

// EditSubmitMsg is emitted only after local validation and contains a minimal
// domain diff. The app layer decides when to invoke Taskwarrior.
type EditSubmitMsg struct {
	Before domain.EditSnapshot
	After  domain.EditSnapshot
	Diff   domain.TaskDiff
}

// EditErrorMsg reports validation without discarding field input. Field
// names the invalid field when HasField is set.
type EditErrorMsg struct {
	Err      error
	Field    EditField
	HasField bool
}

// EditModel is a structured eight-field task editor.
type EditModel struct {
	Inputs          [editFieldCount]textinput.Model
	Task            domain.Task
	Before          domain.EditSnapshot
	Focused         EditField
	Scroll          int
	Open            bool
	Suggestions     []string
	SuggestionIndex int
	SuggestionsOpen bool
	Projects        []string
	ProjectLabels   map[string]string
	Tags            []string
	Width           int
	Height          int
	Location        *time.Location
	Styles          Styles
	Icons           Icons
	Err             error
	ErrField        EditField
	ErrHasField     bool
}

// NewEdit creates a closed editor with initialized Bubbles inputs.
func NewEdit(styles Styles, icons Icons) EditModel {
	model := EditModel{Styles: styles, Icons: icons, Focused: FieldDescription, Location: time.Local}
	for index := range model.Inputs {
		model.Inputs[index] = textinput.New()
		model.Inputs[index].Prompt = ""
		model.Inputs[index].Placeholder = editFieldNames[index]
	}
	model.Inputs[FieldDue].Placeholder = "YYYY-MM-DD HH:MM or tomorrow"
	model.Inputs[FieldScheduled].Placeholder = "YYYY-MM-DD HH:MM or tomorrow"
	model.Inputs[FieldEstimate].Placeholder = "15m, 1h, or 1h30m"
	model.Inputs[FieldRecurrence].Placeholder = "daily, weekdays, weekly, monthly"
	return model
}

// OpenTask loads a task and focuses the requested field directly.
func (e *EditModel) OpenTask(task domain.Task, initial EditField) tea.Cmd {
	if !validField(initial) {
		initial = FieldDescription
	}
	e.Task = task
	e.Before = domain.Snapshot(task)
	e.Before.Due = editableDate(task.Due, e.Before.Due, e.location())
	e.Before.Scheduled = editableDate(task.Scheduled, e.Before.Scheduled, e.location())
	e.Open = true
	e.Err = nil
	e.Scroll = 0
	e.Focused = initial
	estimateValue := ""
	if e.Before.Estimate != nil {
		estimateValue = e.Before.Estimate.InputValue()
	}
	values := [...]string{e.Before.Description, e.Before.Project, e.Before.Priority, e.Before.Due, e.Before.Scheduled, strings.Join(e.Before.Tags, " "), estimateValue, e.Before.Recurrence}
	for index, value := range values {
		e.Inputs[index].SetValue(value)
		e.Inputs[index].Blur()
	}
	e.SetSize(e.Width, e.Height)
	return e.focus(initial)
}

func (e *EditModel) Close() {
	e.Open = false
	e.Scroll = 0
	e.SuggestionsOpen = false
	e.Suggestions = nil
	e.Err = nil
	for index := range e.Inputs {
		e.Inputs[index].Blur()
	}
}

func (e *EditModel) SetSize(width, height int) {
	e.Width, e.Height = width, height
	e.ensureFieldVisible()
}

func (e *EditModel) SetCatalog(projects, tags []string) {
	e.Projects = append([]string(nil), projects...)
	e.Tags = append([]string(nil), tags...)
	e.refreshSuggestions()
}

// SetProjectCatalog installs project values and readable suggestion labels.
func (e *EditModel) SetProjectCatalog(projects domain.ProjectCatalog) {
	e.ProjectLabels = projects.Labels()
	e.SetCatalog(projects.Values(), e.Tags)
}

// Input returns a copy of a field input for read-only inspection.
func (e EditModel) Input(field EditField) textinput.Model {
	if !validField(field) {
		return textinput.Model{}
	}
	return e.Inputs[field]
}

// Update handles modal traversal, field suggestions, and text editing.
func (e *EditModel) Update(msg tea.Msg) (*EditModel, tea.Cmd) {
	if !e.Open {
		return e, nil
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "ctrl+left":
			if e.dateFocused() {
				e.adjustDate(-1, 0)
				return e, nil
			}
		case "ctrl+right":
			if e.dateFocused() {
				e.adjustDate(1, 0)
				return e, nil
			}
		case "ctrl+up":
			if e.dateFocused() {
				e.adjustDate(0, 30*time.Minute)
				return e, nil
			}
		case "ctrl+down":
			if e.dateFocused() {
				e.adjustDate(0, -30*time.Minute)
				return e, nil
			}
		case "esc", "escape":
			e.Close()
			return e, nil
		case "tab", "shift+tab":
			delta := 1
			if keyMsg.String() == "shift+tab" {
				delta = -1
			}
			e.moveField(delta)
			return e, nil
		case "up", "ctrl+p":
			if e.SuggestionsOpen {
				e.moveSuggestion(-1)
			}
			return e, nil
		case "down", "ctrl+n":
			if e.SuggestionsOpen {
				e.moveSuggestion(1)
			}
			return e, nil
		case "enter":
			if e.SuggestionsOpen {
				e.acceptSuggestion()
				return e, nil
			}
		case "ctrl+s":
			return e, e.submit()
		}
	}
	var cmd tea.Cmd
	e.Inputs[e.Focused], cmd = e.Inputs[e.Focused].Update(msg)
	e.Err = nil
	e.ErrHasField = false
	e.refreshSuggestions()
	return e, cmd
}

func (e *EditModel) focus(field EditField) tea.Cmd {
	for index := range e.Inputs {
		if EditField(index) == field {
			continue
		}
		e.Inputs[index].Blur()
	}
	e.Focused = field
	e.ensureFieldVisible()
	return e.Inputs[field].Focus()
}

func (e *EditModel) moveField(delta int) {
	field := int(e.Focused) + delta
	if field < 0 {
		field = len(e.Inputs) - 1
	}
	if field >= len(e.Inputs) {
		field = 0
	}
	e.SuggestionsOpen = false
	e.Suggestions = nil
	e.focus(EditField(field))
	e.refreshSuggestions()
}

func (e *EditModel) refreshSuggestions() {
	if !e.Open {
		e.Suggestions = nil
		e.SuggestionsOpen = false
		return
	}
	var candidates []string
	switch e.Focused {
	case FieldProject:
		candidates = e.Projects
	case FieldTags:
		candidates = e.Tags
	case FieldEstimate:
		candidates = domain.EstimateSuggestions()
	case FieldRecurrence:
		candidates = []string{"daily", "weekdays", "weekly", "monday", "tuesday", "wednesday", "thursday", "friday", "monthly", "2wks"}
	case FieldPriority:
		candidates = []string{"H", "M", "L", "none"}
	case FieldDue, FieldScheduled:
		now := time.Now()
		candidates = []string{"today", "tomorrow", "monday", "tuesday", "wednesday", "thursday", "friday", "next-week", now.Format("2006-01-02")}
	default:
		return
	}
	prefix := strings.TrimSpace(e.Inputs[e.Focused].Value())
	result := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(candidate)]; ok {
			continue
		}
		if prefix != "" && !fuzzyContains(prefix, candidate) {
			if e.Focused != FieldProject || !fuzzyContains(prefix, e.ProjectLabels[candidate]) {
				continue
			}
		}
		seen[strings.ToLower(candidate)] = struct{}{}
		result = append(result, candidate)
	}
	if e.Focused == FieldProject || e.Focused == FieldTags {
		sort.SliceStable(result, func(i, j int) bool { return strings.ToLower(result[i]) < strings.ToLower(result[j]) })
	}
	e.Suggestions = result
	e.SuggestionsOpen = len(result) > 0
	if e.SuggestionIndex >= len(result) {
		e.SuggestionIndex = 0
	}
}

func (e *EditModel) moveSuggestion(delta int) {
	if len(e.Suggestions) == 0 {
		return
	}
	e.SuggestionIndex = (e.SuggestionIndex + delta) % len(e.Suggestions)
	if e.SuggestionIndex < 0 {
		e.SuggestionIndex += len(e.Suggestions)
	}
}

func (e *EditModel) acceptSuggestion() {
	if len(e.Suggestions) == 0 {
		return
	}
	e.Inputs[e.Focused].SetValue(e.Suggestions[e.SuggestionIndex])
	e.Inputs[e.Focused].CursorEnd()
	e.SuggestionsOpen = false
	e.Suggestions = nil
	e.SuggestionIndex = 0
}

func (e *EditModel) submit() tea.Cmd {
	before := e.Before
	description := strings.TrimSpace(e.Inputs[FieldDescription].Value())
	after := e.CurrentSnapshot()
	estimateErr := e.validateEstimateInput()
	return func() tea.Msg {
		if description == "" {
			return EditErrorMsg{Err: fmt.Errorf("description cannot be empty"), Field: FieldDescription, HasField: true}
		}
		switch normalizePriority(after.Priority) {
		case "", "H", "M", "L":
		default:
			return EditErrorMsg{Err: fmt.Errorf("priority must be H, M, L, or empty"), Field: FieldPriority, HasField: true}
		}
		if estimateErr != nil {
			return EditErrorMsg{Err: estimateErr, Field: FieldEstimate, HasField: true}
		}
		if after.Recurrence != "" {
			if recurrence, err := domain.ParseRecurrence(after.Recurrence); err != nil {
				return EditErrorMsg{Err: err, Field: FieldRecurrence, HasField: true}
			} else {
				after.Recurrence = recurrence
				if after.Due == "" {
					return EditErrorMsg{Err: fmt.Errorf("recurring tasks need a first due date"), Field: FieldDue, HasField: true}
				}
			}
		}
		diff := domain.Diff(before, after)
		return EditSubmitMsg{Before: before, After: after, Diff: diff}
	}
}

// CurrentSnapshot returns the current seven field values. Estimate parsing is
// centralized in domain; invalid input is rejected by submit while remaining
// visible in the input control.
func (e EditModel) CurrentSnapshot() domain.EditSnapshot {
	return domain.EditSnapshot{
		Description: e.Inputs[FieldDescription].Value(),
		Project:     e.Inputs[FieldProject].Value(),
		Priority:    normalizePriority(e.Inputs[FieldPriority].Value()),
		Due:         e.Inputs[FieldDue].Value(),
		Scheduled:   e.Inputs[FieldScheduled].Value(),
		Recurrence:  strings.TrimSpace(e.Inputs[FieldRecurrence].Value()),
		Estimate:    e.currentEstimate(),
		Tags:        splitTags(e.Inputs[FieldTags].Value()),
	}
}

func (e EditModel) currentEstimate() *domain.Estimate {
	value := strings.TrimSpace(e.Inputs[FieldEstimate].Value())
	if value == "" {
		return nil
	}
	estimate, err := domain.ParseEstimateValue(value)
	if err != nil {
		return nil
	}
	return &estimate
}

func (e EditModel) validateEstimateInput() error {
	value := strings.TrimSpace(e.Inputs[FieldEstimate].Value())
	if value == "" {
		return nil
	}
	if _, err := domain.ParseEstimate(value); err == nil {
		return nil
	} else if e.Before.Estimate != nil {
		// Values over 24 hours can arrive from another Taskwarrior client. They
		// may remain unchanged or be replaced with a valid Momentum value, but
		// cannot be newly introduced through this editor.
		if parsed, parseErr := domain.ParseEstimateValue(value); parseErr == nil && parsed.Minutes == e.Before.Estimate.Minutes {
			return nil
		}
		return err
	} else {
		return err
	}
}

// ApplyMessage closes after a successful submit or keeps errors in the modal.
func (e *EditModel) ApplyMessage(msg tea.Msg) (tea.Msg, bool) {
	switch message := msg.(type) {
	case EditErrorMsg:
		e.Err = message.Err
		e.ErrField, e.ErrHasField = message.Field, message.HasField
		return nil, true
	case EditSubmitMsg:
		e.Close()
		return message, true
	default:
		return nil, false
	}
}

func (e *EditModel) ensureFieldVisible() {
	if e.Focused < FieldDescription {
		e.Focused = FieldDescription
	}
	if e.Focused >= EditField(editFieldCount) {
		e.Focused = FieldDescription
	}
	// The view derives a window from Focused; Scroll is retained as a small
	// compatibility hint for embedders that inspect editor state.
	e.Scroll = int(e.Focused)
}

// View renders the modal fields with the form-field states: • changed with a
// Muted “was …”, ▌ and a filled input with a cursor cell when focused, and a
// bold Red ! with the message under the value column on error.
func (e EditModel) View() string {
	if !e.Open || e.Width <= 0 || e.Height <= 0 {
		return ""
	}
	icons := e.Icons.orUnicode()
	width := ModalWidth(EditWidth, e.Width)
	content := FrameContentWidth(width)
	narrow := e.Width < NarrowBreakpoint
	labelWidth := 12
	if narrow {
		labelWidth = 10
	}
	valueColumn := 2 + labelWidth + 1
	rows := []FrameRow{{}}
	focusedRow := 0
	changed := 0
	for index := range e.Inputs {
		field := EditField(index)
		label := editFieldNames[index]
		if narrow && field == FieldDescription {
			label = "Title"
		}
		isChanged := e.fieldChanged(field)
		if isChanged {
			changed++
		}
		marker := txt(" ")
		if isChanged {
			marker = sp(icons.Changed, ToneAccent)
		}
		fieldError := e.Err != nil && e.ErrHasField && e.ErrField == field
		if fieldError {
			marker = sp(icons.Error, ToneRed).bold()
		}
		r := FrameRow{}
		if field == e.Focused {
			focusedRow = len(rows)
			input := e.Inputs[index]
			value := inputSpans(input.Value(), input.Position(), content-valueColumn-1, true, input.Placeholder, nil)
			for i := range value {
				value[i].Bg = FillSelection
			}
			r.Mark = icons.Selection
			r.Spans = append([]Span{marker, txt(" "), txt(fmt.Sprintf("%-*s", labelWidth, label)).bold(), {Text: " ", Bg: FillSelection}}, value...)
			r.Spans = append(r.Spans, Span{Text: " ", Grow: true, Bg: FillSelection})
		} else {
			r.Spans = []Span{marker, txt(" "), muted(fmt.Sprintf("%-*s", labelWidth, label)), txt(" ")}
			r.Spans = append(r.Spans, e.fieldValue(field, icons)...)
			if isChanged && !narrow {
				r.Spans = append(r.Spans, muted("   was "+e.beforeValue(field)))
			}
		}
		rows = append(rows, r)
		if fieldError {
			for _, line := range WrapText(e.Err.Error(), max(1, content-valueColumn)) {
				rows = append(rows, row(gap(valueColumn), sp(line, ToneRed)))
			}
		}
		if field == e.Focused {
			if e.SuggestionsOpen {
				rows = append(rows, e.suggestionRows(valueColumn, icons)...)
			}
			if e.dateFocused() {
				guide := "YYYY-MM-DD HH:MM " + icons.Dot + " ctrl+←/→ day " + icons.Dot + " ctrl+↑/↓ 30m"
				if valueColumn+lipgloss.Width(guide) > content {
					rows = append(rows, row(gap(valueColumn), muted("YYYY-MM-DD HH:MM")))
					guide = "ctrl+←/→ day " + icons.Dot + " ctrl+↑/↓ 30m"
				}
				rows = append(rows, row(gap(valueColumn), muted(guide)))
			}
		}
	}
	if e.Err != nil && !e.ErrHasField {
		rows = append(rows, FrameRow{})
		for _, line := range WrapText(e.Err.Error(), max(1, content-2)) {
			rows = append(rows, row(sp(icons.Error, ToneRed).bold(), txt(" "), sp(line, ToneRed)))
		}
	}
	var legend []Span
	if changed > 0 {
		legend = append(legend, sp(icons.Changed, ToneAccent), muted(" changed"))
	}
	if e.Err != nil {
		if len(legend) > 0 {
			legend = append(legend, gap(5))
		}
		legend = append(legend, sp(icons.Error, ToneRed).bold(), muted(" needs a fix before saving"))
	}
	if len(legend) > 0 {
		rows = append(rows, FrameRow{}, row(legend...))
	}
	rows = append(rows, FrameRow{})

	maxRows := ModalMaxRows(e.Height)
	offset := 0
	if len(rows) > maxRows {
		offset = min(max(0, focusedRow-maxRows/2), MaxOffset(len(rows), maxRows))
	}
	var context []Span
	if changed > 0 {
		context = []Span{sp(icons.Changed, ToneAccent), muted(fmt.Sprintf(" %d changed", changed))}
	}
	frame := Frame{
		Title: "Edit task", Context: context, Rows: rows, Offset: offset, Width: width, MaxRows: maxRows,
		Keys: []Hint{{"ctrl+s", "save"}, {"tab", "field"}, {"esc", "cancel"}},
	}
	return strings.Join(e.Styles.RenderFrame(frame, icons), "\n")
}

func (e EditModel) fieldValue(field EditField, icons Icons) []Span {
	value := strings.TrimSpace(e.Inputs[field].Value())
	if value == "" {
		return []Span{muted("none")}
	}
	switch field {
	case FieldProject:
		return []Span{sp("#"+value, ToneCyan)}
	case FieldPriority:
		if label, tone, ok := prioritySlot(value, icons); ok {
			return []Span{sp(strings.Replace(label, " Med", " Medium", 1), tone)}
		}
	case FieldTags:
		return []Span{txt(formatTags(splitTags(value)))}
	}
	return []Span{txt(value)}
}

func (e EditModel) beforeValue(field EditField) string {
	before := e.Before
	value := ""
	switch field {
	case FieldDescription:
		value = before.Description
	case FieldProject:
		value = before.Project
	case FieldPriority:
		value = PriorityLabel(before.Priority)
	case FieldDue:
		value = before.Due
	case FieldScheduled:
		value = before.Scheduled
	case FieldTags:
		value = strings.Join(before.Tags, " ")
	case FieldEstimate:
		if before.Estimate != nil {
			value = before.Estimate.String()
		}
	case FieldRecurrence:
		value = before.Recurrence
	}
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}

func (e EditModel) suggestionRows(column int, icons Icons) []FrameRow {
	labels := append([]string(nil), e.Suggestions...)
	if e.Focused == FieldProject {
		for i, value := range labels {
			if label := e.ProjectLabels[value]; label != "" {
				labels[i] = value + "  " + label
			}
		}
	}
	count := min(3, len(labels))
	start := e.SuggestionIndex - count + 1
	if start < 0 {
		start = 0
	}
	if start+count > len(labels) {
		start = len(labels) - count
	}
	rows := make([]FrameRow, 0, count)
	for index := start; index < start+count; index++ {
		if index == e.SuggestionIndex {
			rows = append(rows, FrameRow{Spans: []Span{gap(column), txt(labels[index]).bold()}, Mark: icons.Selection, Bg: FillSelection})
			continue
		}
		rows = append(rows, row(gap(column), muted(labels[index])))
	}
	return rows
}

func (e EditModel) fieldChanged(field EditField) bool {
	before := e.Before
	after := e.CurrentSnapshot()
	return diffFieldChanged(domain.Diff(before, after), field)
}

func validField(field EditField) bool {
	return field >= FieldDescription && field < EditField(editFieldCount)
}

func (e EditModel) dateFocused() bool {
	return e.Focused == FieldDue || e.Focused == FieldScheduled
}

func (e EditModel) location() *time.Location {
	if e.Location == nil {
		return time.Local
	}
	return e.Location
}

func editableDate(value *time.Time, fallback string, location *time.Location) string {
	if value == nil {
		return fallback
	}
	return value.In(location).Format("2006-01-02 15:04")
}

func (e *EditModel) adjustDate(days int, duration time.Duration) {
	text := strings.TrimSpace(e.Inputs[e.Focused].Value())
	var value time.Time
	var err error
	if text == "" {
		now := time.Now().In(e.location())
		value = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, now.Location())
	} else {
		value, err = parseEditableDate(text, e.location())
		if err != nil {
			e.Err = fmt.Errorf("use YYYY-MM-DD HH:MM before adjusting")
			return
		}
	}
	value = value.AddDate(0, 0, days).Add(duration)
	e.Inputs[e.Focused].SetValue(value.Format("2006-01-02 15:04"))
	e.Inputs[e.Focused].CursorEnd()
	e.Err = nil
	e.refreshSuggestions()
}

func parseEditableDate(value string, location *time.Location) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, nil
		}
	}
	parsed, err := domain.ParseTaskwarriorTime(value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.In(location), nil
}

func normalizePriority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none":
		return ""
	case "h", "high":
		return "H"
	case "m", "medium":
		return "M"
	case "l", "low":
		return "L"
	default:
		return strings.TrimSpace(value)
	}
}

func splitTags(value string) []string {
	parts := strings.Fields(value)
	seen := map[string]struct{}{}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func fuzzyContains(query, candidate string) bool {
	query = strings.ToLower(query)
	candidate = strings.ToLower(candidate)
	if strings.Contains(candidate, query) {
		return true
	}
	queryRunes := []rune(query)
	index := 0
	for _, char := range []rune(candidate) {
		if index < len(queryRunes) && char == queryRunes[index] {
			index++
		}
	}
	return index == len(queryRunes)
}

func diffFieldChanged(d domain.TaskDiff, field EditField) bool {
	switch field {
	case FieldDescription:
		return !d.Description.Empty()
	case FieldProject:
		return !d.Project.Empty()
	case FieldPriority:
		return !d.Priority.Empty()
	case FieldDue:
		return !d.Due.Empty()
	case FieldScheduled:
		return !d.Scheduled.Empty()
	case FieldTags:
		return d.Tags.Changed
	case FieldEstimate:
		return !d.Estimate.Empty()
	case FieldRecurrence:
		return !d.Recurrence.Empty()
	default:
		return false
	}
}
