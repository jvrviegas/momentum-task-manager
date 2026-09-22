package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// PlannerGroup is kept as a string at the UI boundary so the component does
// not own domain/task persistence semantics.
type PlannerGroup string

const (
	PlannerObligation PlannerGroup = "obligation"
	PlannerRollover   PlannerGroup = "rollover"
	PlannerCandidate  PlannerGroup = "candidate"
)

type PlannerItem struct {
	UUID        string
	Description string
	Project     string
	Due         string
	Priority    string
	Estimate    string
	Group       PlannerGroup
	Selected    bool
	Fixed       bool
}

type PlannerEvent struct {
	Summary string
	Start   time.Time
	End     time.Time
	AllDay  bool
}

type PlannerSummary struct {
	ConfiguredMinutes int
	FixedMinutes      int
	CalendarMinutes   int
	BufferMinutes     int
	AvailableMinutes  int
	SelectedMinutes   int
	Unestimated       int
	UnestimatedFixed  int
	MeetingCount      int
	CalendarAvailable bool
	CalendarStale     bool
	CalendarError     string
	CalendarEvents    []PlannerEvent
}

type PlannerSubmitMsg struct{ UUIDs []string }
type PlannerCancelMsg struct{}

// PlannerModel owns the ephemeral daily ritual. It never calls Taskwarrior;
// the app converts the selected UUIDs into idempotent tag mutations.
type PlannerModel struct {
	Open    bool
	Date    time.Time
	Items   []PlannerItem
	Cursor  int
	Scroll  int
	Width   int
	Height  int
	Summary PlannerSummary
	Styles  Styles
	Icons   Icons
	Err     string
}

func NewPlanner(styles Styles, icons Icons) PlannerModel {
	return PlannerModel{Styles: styles, Icons: icons}
}

func (p *PlannerModel) SetSize(width, height int) {
	p.Width, p.Height = width, height
	p.clamp()
}

func (p *PlannerModel) OpenPlan(date time.Time, items []PlannerItem, summary PlannerSummary) {
	p.Open = true
	p.Date = date
	p.Items = append([]PlannerItem(nil), items...)
	p.Summary = summary
	p.Cursor = firstSelectable(items)
	p.Scroll = 0
	p.Err = ""
	p.clamp()
}

func (p *PlannerModel) Close() {
	p.Open = false
	p.Items = nil
	p.Cursor = 0
	p.Scroll = 0
	p.Err = ""
}

func (p *PlannerModel) ApplyCalendar(summary PlannerSummary) {
	p.Summary = summary
}

func (p *PlannerModel) Update(msg tea.Msg) tea.Cmd {
	if !p.Open {
		return nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "esc", "escape":
		p.Close()
		return func() tea.Msg { return PlannerCancelMsg{} }
	case "j", "down":
		p.move(1)
	case "k", "up":
		p.move(-1)
	case "g", "home":
		p.Cursor = firstSelectable(p.Items)
		p.clamp()
	case "G", "end":
		p.Cursor = lastSelectable(p.Items)
		p.clamp()
	case " ", "space":
		p.toggle()
	case "enter", "ctrl+s":
		if p.Err != "" {
			return nil
		}
		return p.submit()
	}
	return nil
}

func (p *PlannerModel) submit() tea.Cmd {
	uuids := p.SelectedUUIDs()
	return func() tea.Msg { return PlannerSubmitMsg{UUIDs: uuids} }
}

func (p *PlannerModel) SelectedUUIDs() []string {
	values := make([]string, 0)
	for _, item := range p.Items {
		if item.Selected && item.UUID != "" {
			values = append(values, item.UUID)
		}
	}
	sort.Strings(values)
	return values
}

func (p *PlannerModel) move(delta int) {
	if len(p.Items) == 0 {
		return
	}
	index := p.Cursor
	if index < 0 || index >= len(p.Items) {
		index = firstSelectable(p.Items)
	}
	for step := 0; step < len(p.Items); step++ {
		index += delta
		if index < 0 {
			index = len(p.Items) - 1
		}
		if index >= len(p.Items) {
			index = 0
		}
		if !p.Items[index].Fixed {
			p.Cursor = index
			p.clamp()
			return
		}
	}
	p.Cursor = index
	p.clamp()
}

func (p *PlannerModel) toggle() {
	if p.Cursor < 0 || p.Cursor >= len(p.Items) || p.Items[p.Cursor].Fixed {
		return
	}
	p.Items[p.Cursor].Selected = !p.Items[p.Cursor].Selected
	p.Summary.SelectedMinutes = 0
	p.Summary.Unestimated = 0
	for _, item := range p.Items {
		if !item.Selected || item.Fixed {
			continue
		}
		if item.Estimate == "" {
			p.Summary.Unestimated++
		}
		p.Summary.SelectedMinutes += parseDisplayMinutes(item.Estimate)
	}
	p.Summary.AvailableMinutes = p.Summary.ConfiguredMinutes - p.Summary.FixedMinutes - p.Summary.CalendarMinutes - p.Summary.BufferMinutes
	p.clamp()
}

func (p *PlannerModel) clamp() {
	if p.Cursor < 0 {
		p.Cursor = 0
	}
	if p.Cursor >= len(p.Items) && len(p.Items) > 0 {
		p.Cursor = len(p.Items) - 1
	}
	if p.Height <= 0 {
		return
	}
	viewport := max(1, p.Height-8-len(p.Summary.CalendarEvents))
	if p.Cursor < p.Scroll {
		p.Scroll = p.Cursor
	}
	if p.Cursor >= p.Scroll+viewport {
		p.Scroll = p.Cursor - viewport + 1
	}
	maxScroll := max(0, len(p.Items)-viewport)
	if p.Scroll > maxScroll {
		p.Scroll = maxScroll
	}
	if p.Scroll < 0 {
		p.Scroll = 0
	}
}

func (p PlannerModel) View() string {
	if !p.Open || p.Width <= 0 || p.Height <= 0 {
		return ""
	}
	width := ModalContentWidth(p.Width, 92)
	height := ModalContentHeight(p.Height, 0)
	lines := make([]string, 0, height)
	lines = append(lines, p.Styles.ModalTitle.Render("Plan today"))
	date := p.Date.Format("Monday, January 2, 2006")
	lines = append(lines, p.Styles.Metadata.Render(Truncate(date+" · Space select · Enter commit · Esc cancel", width)))
	lines = append(lines, p.summaryLine(width))
	if p.Summary.CalendarError != "" {
		lines = append(lines, p.Styles.Error.Render(Truncate("Calendar unavailable: "+p.Summary.CalendarError, width)))
	} else if p.Summary.CalendarStale {
		lines = append(lines, p.Styles.Metadata.Render(Truncate("Calendar data is stale; task planning remains available", width)))
	} else if p.Summary.CalendarAvailable {
		lines = append(lines, p.Styles.Metadata.Render(Truncate(fmt.Sprintf("Calendar: %d meeting(s), %s busy", p.Summary.MeetingCount, formatMinutes(p.Summary.CalendarMinutes)), width)))
		for _, event := range p.Summary.CalendarEvents {
			label := event.Summary
			if label == "" {
				label = "(untitled)"
			}
			when := "All day"
			if !event.AllDay {
				when = event.Start.Format("15:04") + "–" + event.End.Format("15:04")
			}
			lines = append(lines, p.Styles.Metadata.Render(Truncate("  "+when+"  "+label, width)))
		}
	}
	lines = append(lines, "")
	body := p.bodyLines(width)
	viewport := max(1, height-len(lines)-1)
	start := min(p.Scroll, max(0, len(body)-viewport))
	end := min(len(body), start+viewport)
	lines = append(lines, body[start:end]...)
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	lines = append(lines, p.Styles.ModalAction.Render(Truncate("↑/↓ move · Space add/remove · Enter confirm · Esc cancel", width)))
	return renderBoundedPanel(lines, width, height, p.Styles)
}

func (p PlannerModel) bodyLines(width int) []string {
	lines := make([]string, 0, len(p.Items)+6)
	last := PlannerGroup("")
	for index, item := range p.Items {
		if item.Group != last {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			label := "Candidates"
			switch item.Group {
			case PlannerObligation:
				label = "Fixed obligations"
			case PlannerRollover:
				label = "Rollover"
			}
			lines = append(lines, p.Styles.SectionTitle.Render(label))
			last = item.Group
		}
		marker := "[ ]"
		if item.Selected {
			marker = "[x]"
		}
		if item.Fixed {
			marker = "!"
		}
		if index == p.Cursor && !item.Fixed {
			marker = selectionMarker(p.Icons)
			if item.Selected {
				marker += "x"
			}
		}
		meta := make([]string, 0, 4)
		if item.Project != "" {
			meta = append(meta, "#"+item.Project)
		}
		if item.Due != "" {
			meta = append(meta, item.Due)
		}
		if item.Priority != "" {
			meta = append(meta, item.Priority)
		}
		if item.Estimate != "" {
			meta = append(meta, item.Estimate)
		}
		value := item.Description
		if len(meta) > 0 {
			value += "  ·  " + strings.Join(meta, "  ·  ")
		}
		line := fmt.Sprintf("%s %s", marker, value)
		if index == p.Cursor && !item.Fixed {
			line = p.Styles.Selection.Render(PadRight(Truncate(line, width), width))
		} else if item.Fixed {
			line = p.Styles.Metadata.Render(Truncate(line, width))
		} else {
			line = Truncate(line, width)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, p.Styles.Metadata.Render("No pending candidates"))
	}
	return lines
}

func (p PlannerModel) summaryLine(width int) string {
	available := formatMinutes(p.Summary.AvailableMinutes)
	over := p.Summary.SelectedMinutes > p.Summary.AvailableMinutes
	if over {
		available = "over by " + formatMinutes(p.Summary.SelectedMinutes-p.Summary.AvailableMinutes)
	}
	value := fmt.Sprintf("Capacity %s · fixed %s · buffer %s · selected %s · available %s", formatMinutes(p.Summary.ConfiguredMinutes), formatMinutes(p.Summary.FixedMinutes), formatMinutes(p.Summary.BufferMinutes), formatMinutes(p.Summary.SelectedMinutes), available)
	if p.Summary.Unestimated > 0 {
		value += fmt.Sprintf(" · %d unestimated", p.Summary.Unestimated)
	}
	if p.Summary.UnestimatedFixed > 0 {
		value += fmt.Sprintf(" · %d fixed unestimated", p.Summary.UnestimatedFixed)
	}
	if over {
		return p.Styles.Error.Render(Truncate("Warning: over by "+formatMinutes(p.Summary.SelectedMinutes-p.Summary.AvailableMinutes)+" · "+value, width))
	}
	return p.Styles.Status.Render(Truncate(value, width))
}

func firstSelectable(items []PlannerItem) int {
	for index, item := range items {
		if !item.Fixed {
			return index
		}
	}
	if len(items) > 0 {
		return 0
	}
	return -1
}

func lastSelectable(items []PlannerItem) int {
	for index := len(items) - 1; index >= 0; index-- {
		if !items[index].Fixed {
			return index
		}
	}
	return max(0, len(items)-1)
}

func formatMinutes(minutes int) string {
	if minutes < 0 {
		minutes = -minutes
	}
	hours := minutes / 60
	remainder := minutes % 60
	if hours == 0 {
		return fmt.Sprintf("%dm", remainder)
	}
	if remainder == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, remainder)
}

func parseDisplayMinutes(value string) int {
	parts := strings.Fields(strings.ToLower(value))
	minutes := 0
	for _, part := range parts {
		if strings.HasSuffix(part, "h") {
			var hours int
			if _, err := fmt.Sscanf(strings.TrimSuffix(part, "h"), "%d", &hours); err == nil {
				minutes += hours * 60
			}
		} else if strings.HasSuffix(part, "m") {
			var value int
			if _, err := fmt.Sscanf(strings.TrimSuffix(part, "m"), "%d", &value); err == nil {
				minutes += value
			}
		}
	}
	return minutes
}
