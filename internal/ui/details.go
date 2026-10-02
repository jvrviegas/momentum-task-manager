package ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
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
	Now    time.Time
	Styles Styles
	Icons  Icons
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
	width := ModalWidth(DetailsWidth, d.Width)
	return MaxOffset(len(d.bodyRows(FrameContentWidth(width))), ModalMaxRows(d.Height))
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
	width := ModalWidth(DetailsWidth, d.Width)
	keys := []Hint{{"↑↓", "scroll"}, {"e", "edit"}}
	if d.Task.Recurrence != "" {
		keys = append(keys, Hint{"x", "stop recurrence"})
	}
	frame := Frame{
		Title: "Task details", Context: []Span{muted("read-only")},
		Rows: d.bodyRows(FrameContentWidth(width)), Offset: d.Scroll, Width: width, MaxRows: ModalMaxRows(d.Height),
		Keys: append(keys, Hint{"esc", "close"}),
	}
	return strings.Join(d.Styles.RenderFrame(frame, d.Icons), "\n")
}

func (d DetailsModel) now() time.Time {
	if d.Now.IsZero() {
		return time.Now()
	}
	return d.Now
}

// bodyRows groups the record as Overview, Schedule, Notes and Record. ID,
// UUID, urgency and raw properties appear only here.
func (d DetailsModel) bodyRows(width int) []FrameRow {
	icons := d.Icons.orUnicode()
	task := d.Task
	now := d.now()
	rows := []FrameRow{{}}
	for _, line := range WrapText(task.Description, width) {
		rows = append(rows, row(txt(line).bold()))
	}
	if state := taskStateLine(task, now, icons); len(state) > 0 {
		rows = append(rows, row(state...))
	}
	field := func(label string, value ...Span) {
		rows = append(rows, labeledRows(label, value, width)...)
	}
	text := func(value string) []Span {
		if value == "" {
			return []Span{muted("none")}
		}
		return []Span{txt(value)}
	}
	date := func(value *time.Time, raw string) string {
		if value != nil && !value.IsZero() {
			return formatDate(value.In(now.Location()), now)
		}
		return raw
	}

	rows = append(rows, FrameRow{}, sectionRow("Overview", -1, icons))
	if task.Project != "" {
		field("Project", sp("#"+task.Project, ToneCyan))
	} else {
		field("Project", muted("none"))
	}
	if label, tone, ok := prioritySlot(task.Priority, icons); ok {
		field("Priority", sp(strings.Replace(label, " Med", " Medium", 1), tone))
	} else {
		field("Priority", text(strings.TrimSpace(task.Priority))...)
	}
	field("Tags", text(formatTags(task.Tags))...)
	field("Estimate", text(estimateDetail(task))...)

	rows = append(rows, FrameRow{}, sectionRow("Schedule", -1, icons))
	due := text(date(task.Due, task.DueRaw))
	if task.IsPending() && ClassifyOverdue(task, now) {
		due = []Span{sp(icons.Overdue+" Overdue", ToneRed), muted(" " + icons.Dot + " "), sp(date(task.Due, task.DueRaw), ToneRed)}
	}
	field("Due", due...)
	field("Scheduled", text(date(task.Scheduled, task.ScheduledRaw))...)
	if value := date(task.Wait, task.WaitRaw); value != "" {
		field("Wait", txt(value))
	}
	if value := date(task.Start, task.StartRaw); value != "" {
		field("Started", txt(value))
	}
	if value := date(task.End, task.EndRaw); value != "" {
		field("Completed", txt(value))
	}
	if value := date(task.Until, task.UntilRaw); value != "" {
		field("Until", txt(value))
	}
	field("Recurrence", text(recurrenceDetail(task))...)
	if next, ok := recurrenceNext(task); ok {
		next = next.In(now.Location())
		value := formatDate(next, now)
		if next.Year() != now.Year() {
			// A yearly occurrence would otherwise read like this year's date.
			value = next.Format("Jan 2 2006") + strings.TrimPrefix(value, relativeDay(next, now))
		}
		field("Next due", txt(value))
	}

	if len(task.Annotations) > 0 {
		rows = append(rows, FrameRow{}, sectionRow("Notes", len(task.Annotations), icons))
		for index, note := range task.Annotations {
			if index > 0 {
				rows = append(rows, FrameRow{})
			}
			for _, line := range WrapText(note.Description, width) {
				rows = append(rows, row(txt(line)))
			}
			if note.Entry != nil {
				rows = append(rows, row(muted(formatDate(note.Entry.In(now.Location()), now))))
			}
		}
	}

	rows = append(rows, FrameRow{}, sectionRow("Record", -1, icons))
	if task.ID != 0 {
		field("ID", txt(fmt.Sprintf("%d", task.ID)))
	}
	field("UUID", text(task.UUID)...)
	field("Status", text(task.Status)...)
	field("Urgency", txt(fmt.Sprintf("%.2f", task.Urgency)))
	if value := date(task.Entry, task.EntryRaw); value != "" {
		field("Entered", txt(value))
	}
	if value := date(task.Modified, task.ModifiedRaw); value != "" {
		field("Modified", txt(value))
	}
	if len(task.Dependencies) > 0 {
		field("Depends", txt(strings.Join(task.Dependencies, ", ")))
	}
	for _, key := range unknownRawKeys(task.RawFields) {
		field(key, txt(string(task.RawFields[key])))
	}
	return append(rows, FrameRow{})
}

// RenderDetailsPane is the Wide+ read-only preview of the selected task.
func RenderDetailsPane(task *domain.Task, width, height int, now time.Time, styles Styles, icons Icons) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	icons = icons.orUnicode()
	lines := []string{
		styles.Line(width, FillNone, txt("Details").bold(), grow(), muted("enter expands")),
		styles.Line(width, FillNone, rule(icons.Rule)),
		"",
	}
	if task == nil {
		lines = append(lines, styles.Line(width, FillNone, muted("No task selected")))
	} else {
		for _, line := range WrapText(task.Description, width) {
			lines = append(lines, styles.Line(width, FillNone, txt(line).bold()))
		}
		if state := taskStateLine(*task, now, icons); len(state) > 0 {
			lines = append(lines, styles.Line(width, FillNone, state...))
		}
		lines = append(lines, "")
		field := func(label string, value ...Span) {
			for _, r := range labeledRows(label, value, width) {
				lines = append(lines, styles.Line(width, FillNone, r.Spans...))
			}
		}
		if task.Project != "" {
			field("Project", sp("#"+task.Project, ToneCyan))
		}
		if task.Due != nil {
			field("Due", txt(formatDate(task.Due.In(now.Location()), now)))
		}
		if task.Scheduled != nil {
			field("Scheduled", txt(formatDate(task.Scheduled.In(now.Location()), now)))
		}
		if label, tone, ok := prioritySlot(task.Priority, icons); ok {
			field("Priority", sp(strings.Replace(label, " Med", " Medium", 1), tone))
		}
		if len(task.Tags) > 0 {
			field("Tags", txt(formatTags(task.Tags)))
		}
		if value := estimateDetail(*task); value != "" {
			field("Estimate", txt(value))
		}
		if len(task.Annotations) > 0 {
			lines = append(lines, "", styles.Line(width, FillNone, sectionRow("Notes", len(task.Annotations), icons).Spans...))
			for index, note := range task.Annotations {
				if index > 0 {
					lines = append(lines, "")
				}
				for _, line := range WrapText(note.Description, width) {
					lines = append(lines, styles.Line(width, FillNone, txt(line)))
				}
				if note.Entry != nil {
					lines = append(lines, styles.Line(width, FillNone, muted(formatDate(note.Entry.In(now.Location()), now))))
				}
			}
		}
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func taskStateLine(task domain.Task, now time.Time, icons Icons) []Span {
	switch {
	case !task.IsPending() && task.End != nil:
		return []Span{muted(icons.Completed), muted(" Completed " + formatDate(task.End.In(now.Location()), now))}
	case task.Start != nil:
		return []Span{sp(icons.Active, ToneCyan).bold(), txt(" "), sp("Active", ToneCyan), muted(" " + icons.Dot + " started " + task.Start.In(now.Location()).Format("15:04"))}
	default:
		return nil
	}
}

// sectionRow is a Muted caps label, an optional count and a rule.
func sectionRow(title string, count int, icons Icons) FrameRow {
	spans := []Span{muted(strings.ToUpper(title))}
	if count >= 0 {
		spans = append(spans, gap(2), muted(fmt.Sprintf("%d", count)))
	}
	return row(append(spans, gap(2), rule(icons.Rule))...)
}

// labeledRows renders a 12-cell Muted label and a value that wraps under the
// value column.
func labeledRows(label string, value []Span, width int) []FrameRow {
	const labelWidth = 12
	head := muted(fmt.Sprintf("%-*s", labelWidth, Truncate(label, labelWidth-1)))
	if len(value) != 1 || spansWidth(value) <= width-labelWidth || width <= labelWidth {
		return []FrameRow{row(append([]Span{head}, value...)...)}
	}
	lines := WrapText(value[0].Text, width-labelWidth)
	rows := make([]FrameRow, 0, len(lines))
	for index, line := range lines {
		part := value[0]
		part.Text = line
		if index == 0 {
			rows = append(rows, row(head, part))
		} else {
			rows = append(rows, row(gap(labelWidth), part))
		}
	}
	return rows
}

func formatTags(tags []string) string {
	parts := make([]string, 0, len(tags))
	for _, tag := range tags {
		parts = append(parts, "+"+tag)
	}
	return strings.Join(parts, "  ")
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

func estimateDetail(task domain.Task) string {
	if task.Estimate != nil {
		return task.Estimate.String()
	}
	if task.EstimateWarning != "" {
		return "Unavailable: " + task.EstimateWarning
	}
	return ""
}

func recurrenceNext(task domain.Task) (time.Time, bool) {
	if task.Recurrence == "" || task.Due == nil {
		return time.Time{}, false
	}
	return domain.RecurrenceNext(*task.Due, task.Recurrence)
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
