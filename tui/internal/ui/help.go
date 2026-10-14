package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// KeyBinding is the single source for help text and app routing labels.
type KeyBinding struct {
	Category    string
	Keys        string
	Description string
}

var keyBindings = []KeyBinding{
	{Category: "Navigate", Keys: "↑ k", Description: "up"},
	{Category: "Navigate", Keys: "↓ j", Description: "down"},
	{Category: "Navigate", Keys: "g G", Description: "first / last"},
	{Category: "Navigate", Keys: "1-4", Description: "jump to view"},
	{Category: "Navigate", Keys: "h l tab", Description: "sidebar / list"},
	{Category: "Navigate", Keys: "enter", Description: "details"},
	{Category: "Navigate", Keys: "/", Description: "search"},
	{Category: "Navigate", Keys: "esc", Description: "close / back"},
	{Category: "Tasks", Keys: "c", Description: "capture"},
	{Category: "Tasks", Keys: "ctrl+k", Description: "capture"},
	{Category: "Tasks", Keys: "space", Description: "complete"},
	{Category: "Tasks", Keys: "s", Description: "start / stop"},
	{Category: "Tasks", Keys: "e", Description: "edit"},
	{Category: "Tasks", Keys: "p ! D S t", Description: "edit one field"},
	{Category: "Tasks", Keys: "E", Description: "edit estimate"},
	{Category: "Tasks", Keys: "R", Description: "edit recurrence"},
	{Category: "Tasks", Keys: "X", Description: "stop recurrence"},
	{Category: "Tasks", Keys: "P", Description: "plan today"},
	{Category: "Tasks", Keys: "d", Description: "delete"},
	{Category: "Tasks", Keys: "u", Description: "undo"},
	{Category: "App", Keys: "r", Description: "refresh"},
	{Category: "App", Keys: "ctrl+r", Description: "sync now"},
	{Category: "App", Keys: "4", Description: "settings"},
	{Category: "App", Keys: "?", Description: "this help"},
	{Category: "App", Keys: "q", Description: "quit"},
	{Category: "Mouse", Keys: "click", Description: "select a view or a task"},
	{Category: "Mouse", Keys: "wheel", Description: "scroll the list"},
}

// helpKeyWidth is the bold key column; actions follow it.
const helpKeyWidth = 10

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
	width := ModalWidth(HelpWidth, h.Width)
	return MaxOffset(len(helpRows(FrameContentWidth(width))), ModalMaxRows(h.Height))
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

// RenderHelpAt renders a scroll position without holding UI state. Wide
// frames lay Navigate, Tasks and App side by side; narrow frames stack them.
func RenderHelpAt(width, height int, styles Styles, scroll int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	frameWidth := ModalWidth(HelpWidth, width)
	frame := Frame{
		Title: "Keys", Rows: helpRows(FrameContentWidth(frameWidth)), Offset: scroll,
		Width: frameWidth, MaxRows: ModalMaxRows(height), Keys: []Hint{{"esc", "close"}},
	}
	return strings.Join(styles.RenderFrame(frame, Icons{}), "\n")
}

func (h HelpModel) View() string {
	if !h.Open {
		return ""
	}
	return RenderHelpAt(h.Width, h.Height, h.Styles, h.Scroll)
}

func helpGroups() [][]KeyBinding {
	groups := make([][]KeyBinding, 0, 4)
	for _, binding := range KeyBindings() {
		if len(groups) == 0 || groups[len(groups)-1][0].Category != binding.Category {
			groups = append(groups, []KeyBinding{binding})
		} else {
			groups[len(groups)-1] = append(groups[len(groups)-1], binding)
		}
	}
	return groups
}

func helpRows(width int) []FrameRow {
	groups := helpGroups()
	rows := []FrameRow{{}}
	columns := groups
	var rest [][]KeyBinding
	if len(groups) > 3 {
		columns, rest = groups[:3], groups[3:]
	}
	columnWidth := width / 3
	if columnWidth >= helpKeyWidth+15 {
		header := make([]Span, 0, 3)
		depth := 0
		for _, group := range columns {
			header = append(header, muted(fmt.Sprintf("%-*s", columnWidth, strings.ToUpper(group[0].Category))))
			depth = max(depth, len(group))
		}
		rows = append(rows, row(header...))
		for line := 0; line < depth; line++ {
			spans := make([]Span, 0, 6)
			for _, group := range columns {
				key, action := "", ""
				if line < len(group) {
					key, action = group[line].Keys, group[line].Description
				}
				spans = append(spans,
					txt(fmt.Sprintf("%-*s", helpKeyWidth, key)).bold(),
					txt(fmt.Sprintf("%-*s", columnWidth-helpKeyWidth, Truncate(action, columnWidth-helpKeyWidth-1))))
			}
			rows = append(rows, row(spans...))
		}
		groups = rest
		rows = append(rows, FrameRow{})
	}
	for _, group := range groups {
		rows = append(rows, row(muted(strings.ToUpper(group[0].Category))))
		for _, binding := range group {
			for index, line := range WrapText(binding.Description, max(1, width-helpKeyWidth)) {
				key := ""
				if index == 0 {
					key = binding.Keys
				}
				rows = append(rows, row(txt(fmt.Sprintf("%-*s", helpKeyWidth, key)).bold(), txt(line)))
			}
		}
		rows = append(rows, FrameRow{})
	}
	return rows
}

// HelpText returns unstyled generated content for tests and alternate output.
func HelpText() string {
	lines := make([]string, 0, len(keyBindings))
	for _, binding := range KeyBindings() {
		lines = append(lines, binding.Keys+" — "+binding.Description)
	}
	return strings.Join(lines, "\n")
}
