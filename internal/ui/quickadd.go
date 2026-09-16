package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/quickadd"
)

// QuickAddSubmitMsg is emitted after a successful parse; it contains no
// process execution and can be routed to the app command layer.
type QuickAddSubmitMsg struct {
	Task domain.NewTask
}

// QuickAddModel owns the centered capture modal and its contextual suggestions.
type QuickAddModel struct {
	Input           textinput.Model
	Suggestions     []quickadd.Suggestion
	SuggestionIndex int
	SuggestionsOpen bool
	Open            bool
	ParseErr        error
	Projects        []string
	ProjectLabels   map[string]string
	Tags            []string
	Width           int
	Height          int
	Now             time.Time
	Styles          Styles
	Icons           Icons
}

// NewQuickAdd creates a focused quick-capture model.
func NewQuickAdd(styles Styles, icons Icons) QuickAddModel {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Capture a task…"
	return QuickAddModel{Input: input, Styles: styles, Icons: icons, Now: time.Now()}
}

// OpenQuickAdd resets parse state and focuses the command bar.
func (q *QuickAddModel) OpenQuickAdd(value string) tea.Cmd {
	q.Open = true
	q.ParseErr = nil
	q.Input.SetValue(value)
	q.Input.CursorEnd()
	q.refreshSuggestions()
	return q.Input.Focus()
}

// Close closes the bar without touching the underlying Taskwarrior client.
func (q *QuickAddModel) Close() {
	q.Open = false
	q.SuggestionsOpen = false
	q.Suggestions = nil
	q.Input.Blur()
}

func (q *QuickAddModel) SetSize(width, height int) {
	q.Width, q.Height = width, height
	q.Input.SetWidth(max(1, quickAddContentWidth(width)-2))
}

func (q *QuickAddModel) SetCatalog(projects, tags []string) {
	q.Projects = append([]string(nil), projects...)
	q.Tags = append([]string(nil), tags...)
	q.refreshSuggestions()
}

// SetProjectCatalog installs an owned snapshot of project values and labels.
func (q *QuickAddModel) SetProjectCatalog(projects domain.ProjectCatalog) {
	q.ProjectLabels = projects.Labels()
	q.SetCatalog(projects.Values(), q.Tags)
}

// Update routes overlay-owned keys and delegates ordinary editing to Bubbles.
func (q *QuickAddModel) Update(msg tea.Msg) (*QuickAddModel, tea.Cmd) {
	if !q.Open {
		return q, nil
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc", "escape":
			if q.SuggestionsOpen {
				q.SuggestionsOpen = false
				return q, nil
			}
			q.Close()
			return q, nil
		case "tab":
			if q.SuggestionsOpen && len(q.Suggestions) > 0 {
				q.acceptSuggestion()
				return q, nil
			}
		case "up", "ctrl+p":
			if q.SuggestionsOpen {
				q.moveSuggestion(-1)
				return q, nil
			}
		case "down", "ctrl+n":
			if q.SuggestionsOpen {
				q.moveSuggestion(1)
				return q, nil
			}
		case "enter":
			return q, q.submit()
		case "ctrl+k":
			// Ctrl+K is the global opener; inside the bar it must not delete
			// input through textinput's default key map.
			return q, nil
		}
	}
	var cmd tea.Cmd
	q.Input, cmd = q.Input.Update(msg)
	q.ParseErr = nil
	q.refreshSuggestions()
	return q, cmd
}

func (q *QuickAddModel) refreshSuggestions() {
	if !q.Open {
		q.Suggestions = nil
		q.SuggestionsOpen = false
		return
	}
	ctx := quickadd.ContextAt(q.Input.Value(), q.Input.Position())
	q.Suggestions = quickadd.SuggestionsWithProjectLabels(ctx, q.Projects, q.Tags, q.ProjectLabels, q.currentTime())
	q.SuggestionsOpen = len(q.Suggestions) > 0
	if q.SuggestionIndex >= len(q.Suggestions) {
		q.SuggestionIndex = 0
	}
}

func (q *QuickAddModel) currentTime() time.Time {
	if q.Now.IsZero() {
		return time.Now()
	}
	return q.Now
}

func (q *QuickAddModel) moveSuggestion(delta int) {
	if len(q.Suggestions) == 0 {
		return
	}
	q.SuggestionIndex = (q.SuggestionIndex + delta) % len(q.Suggestions)
	if q.SuggestionIndex < 0 {
		q.SuggestionIndex += len(q.Suggestions)
	}
}

func (q *QuickAddModel) acceptSuggestion() {
	if len(q.Suggestions) == 0 {
		return
	}
	ctx := quickadd.ContextAt(q.Input.Value(), q.Input.Position())
	value, cursor := quickadd.ApplySuggestion(q.Input.Value(), ctx, q.Suggestions[q.SuggestionIndex])
	q.Input.SetValue(value)
	q.Input.SetCursor(cursor)
	q.refreshSuggestions()
}

func (q *QuickAddModel) submit() tea.Cmd {
	input := q.Input.Value()
	return func() tea.Msg {
		task, err := quickadd.Parse(input)
		if err != nil {
			return QuickAddErrorMsg{Err: err}
		}
		return QuickAddSubmitMsg{Task: task}
	}
}

// QuickAddErrorMsg keeps the input visible while reporting a parse failure.
type QuickAddErrorMsg struct {
	Err error
}

// ApplyMessage consumes the typed parse result and keeps errors local to the
// overlay. A successful message is returned for the app layer to route.
func (q *QuickAddModel) ApplyMessage(msg tea.Msg) (tea.Msg, bool) {
	switch message := msg.(type) {
	case QuickAddErrorMsg:
		q.ParseErr = message.Err
		return nil, true
	case QuickAddSubmitMsg:
		q.Close()
		return message, true
	default:
		return nil, false
	}
}

// View renders a responsive capture modal with syntax and keyboard guidance.
func (q QuickAddModel) View() string {
	if !q.Open || q.Width <= 0 || q.Height <= 0 {
		return ""
	}
	contentWidth := quickAddContentWidth(q.Width)
	if q.Height <= 2 {
		return q.Styles.Panel.Render(Truncate(q.Input.View(), contentWidth))
	}
	maxLines := max(1, q.Height-2) // Reserve the modal border.

	title := q.Styles.Title.Render("Quick capture")
	bar := q.Styles.Panel.Render(Truncate(q.Input.View(), contentWidth))
	syntax := []string{
		q.Styles.Muted.Render(Truncate("#project · !priority", contentWidth)),
		q.Styles.Muted.Render(Truncate("@due · >scheduled", contentWidth)),
		q.Styles.Muted.Render(Truncate("+tag", contentWidth)),
	}
	keys := q.Styles.Muted.Render(Truncate("Enter add · Esc cancel", contentWidth))

	// Title, input, syntax, and primary keys remain visible whenever the
	// terminal meets the application's minimum size. Suggestions and richer
	// guidance use the remaining space.
	reserved := 6
	if q.ParseErr != nil {
		reserved++
	}
	extra := max(0, maxLines-reserved)
	maxSuggestions := min(5, min(len(q.Suggestions), extra))
	extra -= maxSuggestions

	subtitle := ""
	if extra > 0 {
		subtitle = q.Styles.Muted.Render(Truncate("Describe the task, then add optional metadata.", contentWidth))
		extra--
	}
	example := ""
	if extra > 0 {
		example = q.Styles.Muted.Render(Truncate("Example: Prepare proposal #work !high @tomorrow +planning", contentWidth))
		extra--
	}
	suggestionKeys := ""
	if extra > 0 && q.SuggestionsOpen {
		suggestionKeys = q.Styles.Muted.Render(Truncate("↑/↓ choose · Tab complete", contentWidth))
	}

	lines := []string{title}
	if subtitle != "" {
		lines = append(lines, subtitle)
	}
	lines = append(lines, bar)
	if q.SuggestionsOpen {
		start := max(0, q.SuggestionIndex-maxSuggestions+1)
		for index := start; index < min(len(q.Suggestions), start+maxSuggestions); index++ {
			suggestion := q.Suggestions[index]
			text := suggestion.Text
			if suggestion.Label != "" {
				text = suggestion.Label + "  " + suggestion.Text
			}
			line := fmt.Sprintf("%s %s", q.Icons.Chevron, text)
			if index == q.SuggestionIndex {
				line = q.Styles.Selection.Render(PadRight(Truncate(line, contentWidth), contentWidth))
			} else {
				line = q.Styles.Muted.Render(Truncate(line, contentWidth))
			}
			lines = append(lines, line)
		}
	}
	if q.ParseErr != nil {
		lines = append(lines, q.Styles.Overdue.Render(Truncate(q.ParseErr.Error(), contentWidth)))
	}
	lines = append(lines, syntax...)
	if example != "" {
		lines = append(lines, example)
	}
	if suggestionKeys != "" {
		lines = append(lines, suggestionKeys)
	}
	lines = append(lines, keys)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for index := range lines {
		lines[index] = PadRight(lines[index], contentWidth)
	}
	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return q.Styles.Border.Render(body)
}

func quickAddContentWidth(terminalWidth int) int {
	if terminalWidth <= 0 {
		return 0
	}
	modalWidth := terminalWidth
	if modalWidth > 4 {
		modalWidth -= 4
	}
	if modalWidth > 72 {
		modalWidth = 72
	}
	return max(1, modalWidth-2) // Reserve the modal border.
}

// ErrorText is a plain error accessor for status rendering and tests.
func (q QuickAddModel) ErrorText() string {
	if q.ParseErr == nil {
		return ""
	}
	return strings.TrimSpace(q.ParseErr.Error())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
