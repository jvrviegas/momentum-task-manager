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
	DetailsNone  DetailsAction = "none"
	DetailsClose DetailsAction = "close"
	DetailsEdit  DetailsAction = "edit"
)

// DetailsModel is primarily read-only and deliberately retains the full task
// to display useful raw properties without rewriting them.
type DetailsModel struct {
	Task   domain.Task
	Open   bool
	Width  int
	Height int
	Styles Styles
}

func NewDetails(styles Styles) DetailsModel { return DetailsModel{Styles: styles} }

func (d *DetailsModel) OpenTask(task domain.Task) {
	d.Task = task
	d.Open = true
}

func (d *DetailsModel) Close() { d.Open = false }

func (d *DetailsModel) SetSize(width, height int) { d.Width, d.Height = width, height }

// Update handles only details-owned keys; e moves directly to structured edit.
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
	case "esc", "escape", "enter":
		d.Open = false
		return DetailsClose
	default:
		return DetailsNone
	}
}

func (d DetailsModel) View() string {
	if !d.Open || d.Width <= 0 || d.Height <= 0 {
		return ""
	}
	lines := []string{d.Styles.Title.Render("Task details")}
	appendValue := func(label, value string) {
		if value != "" {
			lines = append(lines, fmt.Sprintf("%-14s %s", label, value))
		}
	}
	appendValue("Description", d.Task.Description)
	appendValue("Status", d.Task.Status)
	appendValue("Project", d.Task.Project)
	appendValue("Priority", d.Task.Priority)
	appendValue("Due", firstNonEmpty(d.Task.DueRaw, formatTaskTime(d.Task.Due)))
	appendValue("Scheduled", firstNonEmpty(d.Task.ScheduledRaw, formatTaskTime(d.Task.Scheduled)))
	appendValue("Start", firstNonEmpty(d.Task.StartRaw, formatTaskTime(d.Task.Start)))
	appendValue("Wait", firstNonEmpty(d.Task.WaitRaw, formatTaskTime(d.Task.Wait)))
	appendValue("Tags", strings.Join(d.Task.Tags, ", "))
	appendValue("Annotations", formatAnnotations(d.Task.Annotations))
	appendValue("Depends", strings.Join(d.Task.Dependencies, ", "))
	appendValue("Recurrence", d.Task.Recurrence)
	appendValue("Urgency", fmt.Sprintf("%.2f", d.Task.Urgency))
	appendValue("UUID", d.Task.UUID)
	if d.Task.ID != 0 {
		appendValue("ID", fmt.Sprintf("%d", d.Task.ID))
	}
	for _, key := range unknownRawKeys(d.Task.RawFields) {
		appendValue(key, string(d.Task.RawFields[key]))
	}
	maxLines := d.Height - 2
	if maxLines < 1 {
		maxLines = 1
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for index := range lines {
		lines[index] = Truncate(lines[index], max(1, d.Width-4))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return d.Styles.Border.Width(max(1, d.Width-2)).Render(body)
}

func unknownRawKeys(fields map[string]json.RawMessage) []string {
	known := map[string]bool{
		"uuid": true, "id": true, "description": true, "status": true, "project": true, "priority": true,
		"entry": true, "end": true, "modified": true, "due": true, "scheduled": true, "start": true, "wait": true,
		"tags": true, "annotations": true, "depends": true, "recur": true, "urgency": true,
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
