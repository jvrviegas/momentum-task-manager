package ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum/internal/domain"
)

// DetailsAction describes how the app should route a details key.
type DetailsAction string

const (
	DetailsNone           DetailsAction = "none"
	DetailsClose          DetailsAction = "close"
	DetailsEdit           DetailsAction = "edit"
	DetailsStopRecurrence DetailsAction = "stop_recurrence"
)

// DetailsModel is primarily read-only and deliberately retains the full task
// to display useful raw properties without rewriting them.
type DetailsModel struct {
	Task   domain.Task
	Open   bool
	Width  int
	Height int
	Scroll int
	Styles Styles
}

func NewDetails(styles Styles) DetailsModel { return DetailsModel{Styles: styles} }

func (d *DetailsModel) OpenTask(task domain.Task) {
	d.Task = task
	d.Open = true
	d.Scroll = 0
}

func (d *DetailsModel) Close() {
	d.Open = false
	d.Scroll = 0
}

func (d *DetailsModel) SetSize(width, height int) {
	d.Width, d.Height = width, height
	d.clampScroll()
}

// Update handles details-owned keys; e moves directly to structured edit and
// arrows/j/k scroll only the modal, never the task list behind it.
func (d *DetailsModel) Update(msg tea.Msg) DetailsAction {
	if !d.Open {
		return DetailsNone
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return DetailsNone
	}
	switch keyMsg.String() {
	case "e":
		d.Open = false
		return DetailsEdit
	case "x", "X":
		if d.Task.Recurrence != "" {
			d.Open = false
			return DetailsStopRecurrence
		}
	case "esc", "escape", "enter":
		d.Close()
		return DetailsClose
	case "j", "down", "ctrl+n", "pagedown":
		d.Scroll++
		d.clampScroll()
	case "k", "up", "ctrl+p", "pageup":
		d.Scroll--
		d.clampScroll()
	case "home", "g":
		d.Scroll = 0
	case "end", "G":
		d.Scroll = d.maxScroll()
	}
	return DetailsNone
}

func (d *DetailsModel) maxScroll() int {
	if !d.Open || d.Width <= 0 || d.Height <= 0 {
		return 0
	}
	body := d.bodyLines(max(1, ModalContentWidth(d.Width, DetailsMaxWidth)))
	viewport := max(1, ModalContentHeight(d.Height, 0)-2)
	return max(0, len(body)-viewport)
}

func (d *DetailsModel) clampScroll() {
	if d.Scroll < 0 {
		d.Scroll = 0
	}
	if maximum := d.maxScroll(); d.Scroll > maximum {
		d.Scroll = maximum
	}
}

func (d DetailsModel) View() string {
	if !d.Open || d.Width <= 0 || d.Height <= 0 {
		return ""
	}
	contentWidth := ModalContentWidth(d.Width, DetailsMaxWidth)
	contentHeight := ModalContentHeight(d.Height, 0)
	body := d.bodyLines(contentWidth)
	viewport := max(1, contentHeight-2) // title and action footer
	scroll := d.Scroll
	maximum := max(0, len(body)-viewport)
	if scroll > maximum {
		scroll = maximum
	}
	if scroll < 0 {
		scroll = 0
	}

	lines := []string{d.Styles.ModalTitle.Render("Task details")}
	end := min(len(body), scroll+viewport)
	if scroll < end {
		lines = append(lines, body[scroll:end]...)
	}
	if maximum > 0 {
		lines = append(lines, d.Styles.ModalAction.Render(fmt.Sprintf("↑/↓ scroll · %d/%d · e edit · x stop recurrence · Esc close", scroll+1, maximum+1)))
	} else {
		lines = append(lines, d.Styles.ModalAction.Render("e edit · x stop recurrence · Esc close"))
	}
	return renderBoundedPanel(lines, contentWidth, contentHeight, d.Styles)
}

func (d DetailsModel) bodyLines(width int) []string {
	lines := make([]string, 0, 24)
	section := func(name string) { lines = append(lines, d.Styles.SectionTitle.Render(name)) }
	appendValue := func(label, value string) {
		if value == "" {
			return
		}
		lines = append(lines, wrapLabeled(label, value, width)...)
	}
	section("CORE")
	appendValue("Description", d.Task.Description)
	appendValue("Status", d.Task.Status)
	appendValue("Priority", PriorityLabel(d.Task.Priority))
	estimateValue := ""
	if d.Task.Estimate != nil {
		estimateValue = d.Task.Estimate.String()
	} else if d.Task.EstimateWarning != "" {
		estimateValue = "Unavailable: " + d.Task.EstimateWarning
	}
	appendValue("Estimate", estimateValue)

	schedule := []struct {
		label string
		value string
	}{
		{"Completed", firstNonEmpty(d.Task.EndRaw, formatTaskTime(d.Task.End))},
		{"Due", firstNonEmpty(d.Task.DueRaw, formatTaskTime(d.Task.Due))},
		{"Scheduled", firstNonEmpty(d.Task.ScheduledRaw, formatTaskTime(d.Task.Scheduled))},
		{"Start", firstNonEmpty(d.Task.StartRaw, formatTaskTime(d.Task.Start))},
		{"Wait", firstNonEmpty(d.Task.WaitRaw, formatTaskTime(d.Task.Wait))},
		{"Until", firstNonEmpty(d.Task.UntilRaw, formatTaskTime(d.Task.Until))},
	}
	hasSchedule := false
	for _, field := range schedule {
		if field.value != "" {
			hasSchedule = true
			break
		}
	}
	if hasSchedule {
		section("SCHEDULE")
		for _, field := range schedule {
			appendValue(field.label, field.value)
		}
	}

	organizationValues := []struct {
		label string
		value string
	}{
		{"Project", d.Task.Project},
		{"Tags", strings.Join(d.Task.Tags, ", ")},
		{"Annotations", formatAnnotations(d.Task.Annotations)},
		{"Depends", strings.Join(d.Task.Dependencies, ", ")},
		{"Recurrence", recurrenceDetail(d.Task)},
		{"Next occurrence", recurrenceNextDetail(d.Task)},
	}
	hasOrganization := false
	for _, field := range organizationValues {
		if field.value != "" {
			hasOrganization = true
			break
		}
	}
	if hasOrganization {
		section("ORGANIZATION")
		for _, field := range organizationValues {
			appendValue(field.label, field.value)
		}
	}

	section("TECHNICAL")
	appendValue("Urgency", fmt.Sprintf("%.2f", d.Task.Urgency))
	appendValue("UUID", d.Task.UUID)
	if d.Task.ID != 0 {
		appendValue("ID", fmt.Sprintf("%d", d.Task.ID))
	}
	for _, key := range unknownRawKeys(d.Task.RawFields) {
		appendValue(key, string(d.Task.RawFields[key]))
	}
	return lines
}

func wrapLabeled(label, value string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	prefix := label + ": "
	if lipgloss.Width(prefix) >= width {
		return WrapText(prefix+value, width)
	}
	valueWidth := width - lipgloss.Width(prefix)
	wrapped := WrapText(value, valueWidth)
	lines := make([]string, 0, len(wrapped))
	continuation := strings.Repeat(" ", lipgloss.Width(prefix))
	for index, line := range wrapped {
		if index == 0 {
			lines = append(lines, prefix+line)
		} else {
			lines = append(lines, continuation+line)
		}
	}
	return lines
}

func unknownRawKeys(fields map[string]json.RawMessage) []string {
	known := map[string]bool{
		"uuid": true, "id": true, "description": true, "status": true, "project": true, "priority": true,
		"entry": true, "end": true, "modified": true, "due": true, "scheduled": true, "start": true, "wait": true, "until": true,
		"tags": true, "annotations": true, "depends": true, "recur": true, "parent": true, "mask": true, "imask": true, "rtype": true, "urgency": true, "estimate": true,
	}
	keys := make([]string, 0)
	for key := range fields {
		if !known[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func formatTaskTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func recurrenceNextDetail(task domain.Task) string {
	if task.Recurrence == "" || task.Due == nil {
		return ""
	}
	if next, ok := domain.RecurrenceNext(*task.Due, task.Recurrence); ok {
		return next.Format(time.RFC3339)
	}
	return ""
}

func recurrenceDetail(task domain.Task) string {
	if task.Recurrence == "" {
		return ""
	}
	value := domain.RecurrenceDisplay(task.Recurrence) + " [" + task.Recurrence + "]"
	if task.IsRecurrenceTemplate() {
		return value + " (template)"
	}
	if task.IsRecurrenceInstance() {
		return value + " (generated instance; parent " + task.Parent + ")"
	}
	return value
}

func formatAnnotations(values []domain.Annotation) string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value.Description != "" {
			result = append(result, value.Description)
		}
	}
	return strings.Join(result, "; ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
