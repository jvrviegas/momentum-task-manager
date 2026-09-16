package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	Open            bool
	Suggestions     []string
	SuggestionIndex int
	SuggestionsOpen bool
	Projects        []string
	Tags            []string
	Width           int
	Height          int
	Styles          Styles
	Icons           Icons
	Err             error
}

// NewEdit creates a closed editor with initialized Bubbles inputs.
func NewEdit(styles Styles, icons Icons) EditModel {
	model := EditModel{Styles: styles, Icons: icons, Focused: FieldDescription}
	for index := range model.Inputs {
		model.Inputs[index] = textinput.New()
		model.Inputs[index].Prompt = ""
		model.Inputs[index].Placeholder = editFieldNames[index]
	}
	return model
}

// OpenTask loads a task and focuses the requested field directly.
func (e *EditModel) OpenTask(task domain.Task, initial EditField) tea.Cmd {
	if !validField(initial) {
		initial = FieldDescription
	}
	e.Task = task
	e.Before = domain.Snapshot(task)
	e.Open = true
	e.Err = nil
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
	e.SuggestionsOpen = false
	e.Suggestions = nil
	e.Err = nil
	for index := range e.Inputs {
		e.Inputs[index].Blur()
	}
}

func (e *EditModel) SetSize(width, height int) {
	e.Width, e.Height = width, height
	for index := range e.Inputs {
		e.Inputs[index].SetWidth(max(1, width-18))
	}
}

func (e *EditModel) SetCatalog(projects, tags []string) {
	e.Projects = append([]string(nil), projects...)
	e.Tags = append([]string(nil), tags...)
	e.refreshSuggestions()
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
		case "esc", "escape":
			e.Close()
			return e, nil
		case "tab", "shift+tab":
			if e.SuggestionsOpen && keyMsg.String() == "tab" {
				e.acceptSuggestion()
				return e, nil
			}
			delta := 1
			if keyMsg.String() == "shift+tab" {
				delta = -1
			}
			e.moveField(delta)
			return e, nil
		case "up", "ctrl+p":
			if e.SuggestionsOpen {
				e.moveSuggestion(-1)
			} else {
				e.moveField(-1)
			}
			return e, nil
		case "down", "ctrl+n":
			if e.SuggestionsOpen {
				e.moveSuggestion(1)
			} else {
				e.moveField(1)
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
			continue
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
	e.refreshSuggestions()
}

func (e *EditModel) submit() tea.Cmd {
	before := e.Before
	after := e.CurrentSnapshot()
	return func() tea.Msg {
		if strings.TrimSpace(after.Description) == "" {
			return EditErrorMsg{Err: fmt.Errorf("description cannot be empty")}
		}
		switch strings.ToUpper(strings.TrimSpace(after.Priority)) {
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
		Priority:    e.Inputs[FieldPriority].Value(),
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
	lines := []string{e.Styles.Title.Render("Edit task")}
	for index, name := range editFieldNames {
		value := e.Inputs[index].View()
		marker := " "
		if e.fieldChanged(EditField(index)) {
			marker = "*"
		}
		line := fmt.Sprintf("%s %-10s %s", marker, name, value)
		if EditField(index) == e.Focused {
			line = e.Styles.Selection.Render(Truncate(line, e.Width))
		} else {
			line = Truncate(line, e.Width)
		}
		lines = append(lines, line)
	}
	if e.SuggestionsOpen {
		lines = append(lines, e.Styles.Muted.Render(Truncate("Suggestions: "+strings.Join(e.Suggestions, "  "), e.Width)))
	}
	if e.Err != nil {
		lines = append(lines, e.Styles.Overdue.Render(Truncate(e.Err.Error(), e.Width)))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return e.Styles.Border.Width(max(1, e.Width-2)).Render(body)
}

func (e EditModel) fieldChanged(field EditField) bool {
	before := e.Before
	after := e.CurrentSnapshot()
	return diffFieldChanged(domain.Diff(before, after), field)
}

func validField(field EditField) bool { return field >= FieldDescription && field <= FieldTags }

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
