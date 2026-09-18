package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
)

// EditField identifies one of the six supported editable fields.
type EditField int

const (
	FieldDescription EditField = iota
	FieldProject
	FieldPriority
	FieldDue
	FieldScheduled
	FieldTags
)

var editFieldNames = [...]string{"Description", "Project", "Priority", "Due", "Scheduled", "Tags"}

// EditSubmitMsg is emitted only after local validation and contains a minimal
// domain diff. The app layer decides when to invoke Taskwarrior.
type EditSubmitMsg struct {
	Before domain.EditSnapshot
	After  domain.EditSnapshot
	Diff   domain.TaskDiff
}

// EditErrorMsg reports validation without discarding field input.
type EditErrorMsg struct{ Err error }

// EditModel is a structured six-field task editor.
type EditModel struct {
	Inputs          [6]textinput.Model
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
	values := []string{e.Before.Description, e.Before.Project, e.Before.Priority, e.Before.Due, e.Before.Scheduled, strings.Join(e.Before.Tags, " ")}
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
	contentWidth := ModalContentWidth(width, ModalMaxWidth)
	for index := range e.Inputs {
		e.Inputs[index].SetWidth(max(1, contentWidth-18))
	}
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
	after := e.CurrentSnapshot()
	return func() tea.Msg {
		if strings.TrimSpace(after.Description) == "" {
			return EditErrorMsg{Err: fmt.Errorf("description cannot be empty")}
		}
		switch normalizePriority(after.Priority) {
		case "", "H", "M", "L":
		default:
			return EditErrorMsg{Err: fmt.Errorf("priority must be H, M, L, or empty")}
		}
		diff := domain.Diff(before, after)
		return EditSubmitMsg{Before: before, After: after, Diff: diff}
	}
}

// CurrentSnapshot returns the current six field values.
func (e EditModel) CurrentSnapshot() domain.EditSnapshot {
	return domain.EditSnapshot{
		Description: e.Inputs[FieldDescription].Value(),
		Project:     e.Inputs[FieldProject].Value(),
		Priority:    normalizePriority(e.Inputs[FieldPriority].Value()),
		Due:         e.Inputs[FieldDue].Value(),
		Scheduled:   e.Inputs[FieldScheduled].Value(),
		Tags:        splitTags(e.Inputs[FieldTags].Value()),
	}
}

// ApplyMessage closes after a successful submit or keeps errors in the modal.
func (e *EditModel) ApplyMessage(msg tea.Msg) (tea.Msg, bool) {
	switch message := msg.(type) {
	case EditErrorMsg:
		e.Err = message.Err
		return nil, true
	case EditSubmitMsg:
		e.Close()
		return message, true
	default:
		return nil, false
	}
}

// View renders the modal fields, focused field, changed indicators, and
// contextual suggestions.
func (e EditModel) View() string {
	if !e.Open || e.Width <= 0 || e.Height <= 0 {
		return ""
	}
	contentWidth := ModalContentWidth(e.Width, ModalMaxWidth)
	contentHeight := ModalContentHeight(e.Height, 0)

	fieldBudget := contentHeight - 2 // title and action footer
	if e.Err != nil {
		fieldBudget--
	}
	if e.SuggestionsOpen {
		fieldBudget--
	}
	if e.dateFocused() {
		fieldBudget--
	}
	if fieldBudget < 1 {
		fieldBudget = 1
	}
	start, end := e.fieldWindow(fieldBudget)
	lines := []string{e.Styles.ModalTitle.Render("Edit task")}
	for index := start; index < end; index++ {
		field := EditField(index)
		marker := " "
		if e.fieldChanged(field) {
			marker = "*"
		}
		if field == e.Focused {
			marker = selectionMarker(e.Icons)
		}
		value := e.Inputs[index].View()
		line := fmt.Sprintf("%s %-10s %s", marker, e.Styles.FieldLabel.Render(editFieldNames[index]), value)
		if field == e.Focused {
			line = e.Styles.Selection.Render(PadRight(Truncate(line, contentWidth), contentWidth))
		} else {
			line = Truncate(line, contentWidth)
		}
		lines = append(lines, line)
	}
	if e.SuggestionsOpen {
		lines = append(lines, e.renderSuggestions(contentWidth)...)
	}
	if e.dateFocused() {
		guidance := "Date: YYYY-MM-DD HH:MM · Ctrl+←/→ day · Ctrl+↑/↓ 30m"
		lines = append(lines, e.Styles.Metadata.Render(Truncate(guidance, contentWidth)))
	}
	if e.Err != nil {
		lines = append(lines, e.Styles.Error.Render(Truncate(e.Err.Error(), contentWidth)))
	}
	lines = append(lines, e.Styles.ModalAction.Render(Truncate("Tab next · Ctrl+S save · Esc cancel", contentWidth)))
	return renderBoundedPanel(lines, contentWidth, contentHeight, e.Styles)
}

func (e EditModel) fieldWindow(budget int) (int, int) {
	if budget >= len(editFieldNames) {
		return 0, len(editFieldNames)
	}
	start := int(e.Focused) - budget + 1
	if start < 0 {
		start = 0
	}
	end := start + budget
	if end > len(editFieldNames) {
		end = len(editFieldNames)
		start = end - budget
	}
	return start, end
}

func (e *EditModel) ensureFieldVisible() {
	if e.Focused < FieldDescription {
		e.Focused = FieldDescription
	}
	if e.Focused > FieldTags {
		e.Focused = FieldTags
	}
	// The view derives a window from Focused; Scroll is retained as a small
	// compatibility hint for embedders that inspect editor state.
	e.Scroll = int(e.Focused)
}

func (e EditModel) renderSuggestions(width int) []string {
	labels := append([]string(nil), e.Suggestions...)
	if e.Focused == FieldProject {
		for i, value := range labels {
			if label := e.ProjectLabels[value]; label != "" {
				labels[i] = label + " (" + value + ")"
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
	lines := make([]string, 0, count)
	for index := start; index < start+count; index++ {
		marker := " "
		if index == e.SuggestionIndex {
			marker = selectionMarker(e.Icons)
		}
		lines = append(lines, e.Styles.Metadata.Render(Truncate(fmt.Sprintf("Suggestions %s %s", marker, labels[index]), width)))
	}
	return lines
}

func (e EditModel) fieldChanged(field EditField) bool {
	before := e.Before
	after := e.CurrentSnapshot()
	return diffFieldChanged(domain.Diff(before, after), field)
}

func validField(field EditField) bool { return field >= FieldDescription && field <= FieldTags }

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
	default:
		return false
	}
}
