package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

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
	input.Placeholder = "What needs doing?"
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
	// Bubbles reserves one additional cursor cell beyond the two-cell prompt.
	// Budget it here so an idle input never gains a misleading trailing ellipsis.
	q.Input.SetWidth(max(1, quickAddContentWidth(width)-3))
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
	contentHeight := ModalContentHeight(q.Height, 0)
	if contentHeight < 1 {
		contentHeight = 1
	}

	title := q.Styles.ModalTitle.Render("Quick capture")
	label := q.Styles.FieldLabel.Render(Truncate("Task", contentWidth))
	bar := q.Styles.Panel.Render(Truncate(q.Input.View(), contentWidth))
	keys := q.Styles.ModalAction.Render(Truncate("Enter add task · Esc cancel", contentWidth))

	// Keep the capture path visually singular: title, question, input, action.
	// Contextual suggestions replace (rather than stack on top of) the syntax
	// guide. This avoids turning the modal into a wall of equally weighted help.
	mandatory := 4 // title, label, input, actions
	if q.ParseErr != nil {
		mandatory++
	}
	optional := max(0, contentHeight-mandatory)
	lines := []string{title, label, bar}
	if q.ParseErr != nil {
		lines = append(lines, q.Styles.Error.Render(Truncate(q.ParseErr.Error(), contentWidth)))
	}

	if q.ParseErr == nil && q.SuggestionsOpen && len(q.Suggestions) > 0 && optional > 0 {
		heading := suggestionHeading(q.Suggestions[0].Kind) + "  ·  ↑/↓ select  ·  Tab use"
		lines = append(lines, q.Styles.SectionTitle.Render(Truncate(heading, contentWidth)))
		optional--
		maxSuggestions := min(5, min(len(q.Suggestions), optional))
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
				line = q.Styles.Metadata.Render(Truncate(line, contentWidth))
			}
			lines = append(lines, line)
		}
	} else if q.ParseErr == nil && optional >= 2 {
		guide := []string{
			q.Styles.SectionTitle.Render(Truncate("Optional details", contentWidth)),
			q.Styles.Metadata.Render(Truncate("#project   !priority   @due date", contentWidth)),
			q.Styles.Metadata.Render(Truncate(">scheduled   +tag", contentWidth)),
		}
		lines = append(lines, guide[:min(len(guide), optional)]...)
	}

	lines = append(lines, keys)
	return renderBoundedPanel(lines, contentWidth, contentHeight, q.Styles)
}

func suggestionHeading(kind quickadd.SuggestionKind) string {
	switch kind {
	case quickadd.SuggestionProject:
		return "Projects"
	case quickadd.SuggestionPriority:
		return "Priorities"
	case quickadd.SuggestionDue:
		return "Due dates"
	case quickadd.SuggestionScheduled:
		return "Scheduled dates"
	case quickadd.SuggestionTag:
		return "Tags"
	default:
		return "Suggestions"
	}
}

func quickAddContentWidth(terminalWidth int) int {
	return ModalContentWidth(terminalWidth, QuickAddMaxWidth)
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
