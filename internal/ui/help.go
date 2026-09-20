package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// KeyBinding is the single source for help text and app routing labels.
type KeyBinding struct {
	Category    string
	Keys        string
	Description string
}

var keyBindings = []KeyBinding{
	{Category: "Navigation", Keys: "j / k / ↑ / ↓", Description: "move selection"},
	{Category: "Navigation", Keys: "h / l / Tab", Description: "switch focus/view"},
	{Category: "Navigation", Keys: "g / G", Description: "first / last task"},
	{Category: "Navigation", Keys: "1 / 2 / 3 / 4", Description: "Inbox / Today / Completed / Settings"},
	{Category: "Tasks", Keys: "Enter", Description: "open details"},
	{Category: "Tasks", Keys: "Space", Description: "complete task"},
	{Category: "Tasks", Keys: "e / p / ! / D / S / t", Description: "edit field"},
	{Category: "Tasks", Keys: "s", Description: "start / stop task"},
	{Category: "Tasks", Keys: "d", Description: "delete task"},
	{Category: "Tasks", Keys: "u", Description: "undo last change"},
	{Category: "Search / Create / Edit", Keys: "c / Ctrl+K", Description: "create task (quick add)"},
	{Category: "Search / Create / Edit", Keys: "/", Description: "search (project:name filters project)"},
	{Category: "Application", Keys: "r", Description: "refresh tasks"},
	{Category: "Application", Keys: "Ctrl+R", Description: "synchronize now"},
	{Category: "Application", Keys: "?", Description: "show help"},
	{Category: "Application", Keys: "q", Description: "quit"},
	{Category: "Application", Keys: "Esc", Description: "close active overlay"},
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

// HelpModel renders generated key help and owns its local scroll position.
type HelpModel struct {
	Open   bool
	Width  int
	Height int
	Scroll int
	Styles Styles
}

func NewHelp(styles Styles) HelpModel { return HelpModel{Styles: styles} }
func (h *HelpModel) SetSize(width, height int) {
	h.Width, h.Height = width, height
	h.clampScroll()
}
func (h *HelpModel) OpenHelp() {
	h.Open = true
	h.Scroll = 0
}
func (h *HelpModel) Close() {
	h.Open = false
	h.Scroll = 0
}

func (h *HelpModel) Update(msg tea.Msg) HelpAction {
	if !h.Open {
		return HelpNone
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return HelpNone
	}
	switch keyMsg.String() {
	case "esc", "escape", "?":
		h.Close()
		return HelpClose
	case "j", "down", "ctrl+n", "pagedown":
		h.Scroll++
		h.clampScroll()
	case "k", "up", "ctrl+p", "pageup":
		h.Scroll--
		h.clampScroll()
	case "home":
		h.Scroll = 0
	case "end", "G":
		h.Scroll = h.maxScroll()
	}
	return HelpNone
}

func (h *HelpModel) maxScroll() int {
	if !h.Open || h.Width <= 0 || h.Height <= 0 {
		return 0
	}
	body := helpBodyLines(ModalContentWidth(h.Width, HelpMaxWidth), h.Styles)
	viewport := max(1, ModalContentHeight(h.Height, 0)-2)
	return max(0, len(body)-viewport)
}

func (h *HelpModel) clampScroll() {
	if h.Scroll < 0 {
		h.Scroll = 0
	}
	if maximum := h.maxScroll(); h.Scroll > maximum {
		h.Scroll = maximum
	}
}

// RenderHelp renders the initial page of generated key help.
func RenderHelp(width, height int, styles Styles) string {
	return RenderHelpAt(width, height, styles, 0)
}

// RenderHelpAt renders a scroll position without holding UI state. The
// category-aware layout uses two columns only when each column has enough room
// for readable keys and descriptions.
func RenderHelpAt(width, height int, styles Styles, scroll int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	contentWidth := ModalContentWidth(width, HelpMaxWidth)
	contentHeight := ModalContentHeight(height, 0)
	body := helpBodyLines(contentWidth, styles)
	viewport := max(1, contentHeight-2)
	maximum := max(0, len(body)-viewport)
	if scroll < 0 {
		scroll = 0
	}
	if scroll > maximum {
		scroll = maximum
	}

	lines := []string{styles.ModalTitle.Render("Keyboard shortcuts")}
	end := min(len(body), scroll+viewport)
	if scroll < end {
		lines = append(lines, body[scroll:end]...)
	}
	if maximum > 0 {
		lines = append(lines, styles.ModalAction.Render(fmt.Sprintf("↑/↓ scroll · %d/%d · Esc close", scroll+1, maximum+1)))
	} else {
		lines = append(lines, styles.ModalAction.Render("Esc close"))
	}
	return renderBoundedPanel(lines, contentWidth, contentHeight, styles)
}

func (h HelpModel) View() string {
	if !h.Open {
		return ""
	}
	return RenderHelpAt(h.Width, h.Height, h.Styles, h.Scroll)
}

func helpBodyLines(width int, styles Styles) []string {
	if width >= 64 {
		return helpTwoColumnLines(width, styles)
	}
	return helpSingleColumnLines(width, styles)
}

func helpSingleColumnLines(width int, styles Styles) []string {
	lines := make([]string, 0, len(keyBindings)+4)
	lastCategory := ""
	for _, binding := range KeyBindings() {
		if binding.Category != lastCategory {
			lines = append(lines, styles.SectionTitle.Render(binding.Category))
			lastCategory = binding.Category
		}
		lines = append(lines, helpBindingLines(binding, width, styles)...)
	}
	return lines
}

func helpTwoColumnLines(width int, styles Styles) []string {
	groups := make([][]KeyBinding, 0, 4)
	for _, binding := range KeyBindings() {
		if len(groups) == 0 || groups[len(groups)-1][0].Category != binding.Category {
			groups = append(groups, []KeyBinding{binding})
		} else {
			groups[len(groups)-1] = append(groups[len(groups)-1], binding)
		}
	}
	leftGroups := groups[:2]
	rightGroups := groups[2:]
	left := helpColumnLines(leftGroups, (width-3)/2, styles)
	right := helpColumnLines(rightGroups, (width-3)/2, styles)
	rows := max(len(left), len(right))
	columnWidth := (width - 3) / 2
	lines := make([]string, 0, rows)
	for index := 0; index < rows; index++ {
		leftLine, rightLine := "", ""
		if index < len(left) {
			leftLine = left[index]
		}
		if index < len(right) {
			rightLine = right[index]
		}
		leftLine = PadRight(Truncate(leftLine, columnWidth), columnWidth)
		if rightLine == "" {
			lines = append(lines, leftLine)
		} else {
			lines = append(lines, leftLine+"   "+Truncate(rightLine, columnWidth))
		}
	}
	return lines
}

func helpColumnLines(groups [][]KeyBinding, width int, styles Styles) []string {
	lines := make([]string, 0)
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		lines = append(lines, styles.SectionTitle.Render(group[0].Category))
		for _, binding := range group {
			lines = append(lines, helpBindingLines(binding, width, styles)...)
		}
	}
	return lines
}

func helpBindingLines(binding KeyBinding, width int, _ Styles) []string {
	keyWidth := 18
	if width < 30 {
		// Keep enough room for complete words in narrow descriptions; a
		// very wide key column makes the help screen look truncated even
		// though scrolling is available.
		keyWidth = max(1, width/3)
	} else if width < keyWidth+4 {
		keyWidth = max(1, width/2)
	}
	prefix := fmt.Sprintf("%-*s ", keyWidth, binding.Keys)
	available := width - lipgloss.Width(prefix)
	if available < 1 {
		available = 1
	}
	wrapped := WrapText(binding.Description, available)
	if len(wrapped) == 0 {
		return []string{Truncate(prefix, width)}
	}
	lines := make([]string, 0, len(wrapped))
	for index, value := range wrapped {
		if index == 0 {
			lines = append(lines, Truncate(prefix+value, width))
		} else {
			lines = append(lines, Truncate(strings.Repeat(" ", lipgloss.Width(prefix))+value, width))
		}
	}
	return lines
}

// HelpText returns unstyled generated content for tests and alternate output.
func HelpText() string {
	lines := make([]string, 0, len(keyBindings))
	for _, binding := range KeyBindings() {
		lines = append(lines, binding.Keys+" — "+binding.Description)
	}
	return strings.Join(lines, "\n")
}
