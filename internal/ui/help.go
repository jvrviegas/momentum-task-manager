package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// KeyBinding is the single source for help text and app routing labels.
type KeyBinding struct {
	Keys        string
	Description string
}

var keyBindings = []KeyBinding{
	{Keys: "j / k / ↑ / ↓", Description: "move selection"},
	{Keys: "h / l / Tab", Description: "switch focus/view"},
	{Keys: "g / G", Description: "first / last task"},
	{Keys: "1 / 2 / 3", Description: "Inbox / Today / Settings"},
	{Keys: "Enter", Description: "open details"},
	{Keys: "Ctrl+K", Description: "quick add"},
	{Keys: "/", Description: "search (project:name filters project)"},
	{Keys: "Space", Description: "complete task"},
	{Keys: "e / p / ! / D / S / t", Description: "edit field"},
	{Keys: "s", Description: "start / stop task"},
	{Keys: "d", Description: "delete task"},
	{Keys: "u", Description: "undo last change"},
	{Keys: "r", Description: "refresh tasks"},
	{Keys: "Ctrl+R", Description: "synchronize now"},
	{Keys: "?", Description: "show help"},
	{Keys: "q", Description: "quit"},
	{Keys: "Esc", Description: "close active overlay"},
}

// KeyBindings returns a copy so callers cannot drift help from routing by
// mutating the package's definitions.
func KeyBindings() []KeyBinding {
	return append([]KeyBinding(nil), keyBindings...)
}

// HelpAction is the small lifecycle result for the help overlay.
type HelpAction string

const (
	HelpNone  HelpAction = "none"
	HelpClose HelpAction = "close"
)

// HelpModel renders generated key help.
type HelpModel struct {
	Open   bool
	Width  int
	Height int
	Styles Styles
}

func NewHelp(styles Styles) HelpModel          { return HelpModel{Styles: styles} }
func (h *HelpModel) SetSize(width, height int) { h.Width, h.Height = width, height }
func (h *HelpModel) OpenHelp()                 { h.Open = true }
func (h *HelpModel) Close()                    { h.Open = false }

func (h *HelpModel) Update(msg tea.Msg) HelpAction {
	if !h.Open {
		return HelpNone
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return HelpNone
	}
	if keyMsg.String() == "esc" || keyMsg.String() == "escape" || keyMsg.String() == "?" {
		h.Close()
		return HelpClose
	}
	return HelpNone
}

func RenderHelp(width, height int, styles Styles) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := []string{styles.Title.Render("Keyboard shortcuts")}
	for _, binding := range KeyBindings() {
		line := fmt.Sprintf("%-24s %s", binding.Keys, binding.Description)
		lines = append(lines, Truncate(line, max(1, width-4)))
	}
	lines = append(lines, styles.Muted.Render("Esc to close"))
	maxLines := max(1, height-2)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return styles.Border.Width(max(1, width-2)).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (h HelpModel) View() string {
	if !h.Open {
		return ""
	}
	return RenderHelp(h.Width, h.Height, h.Styles)
}

// HelpText returns unstyled generated content for tests and alternate output.
func HelpText() string {
	lines := make([]string, 0, len(keyBindings))
	for _, binding := range KeyBindings() {
		lines = append(lines, binding.Keys+" — "+binding.Description)
	}
	return strings.Join(lines, "\n")
}
