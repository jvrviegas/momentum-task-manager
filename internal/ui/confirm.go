package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

// ConfirmAction is the only result a destructive confirmation can emit.
type ConfirmAction string

const (
	ConfirmNone   ConfirmAction = "none"
	ConfirmYes    ConfirmAction = "yes"
	ConfirmNo     ConfirmAction = "no"
	ConfirmCancel ConfirmAction = "cancel"
)

// ConfirmModel owns an explicit y/n modal. A task confirmation is a danger
// frame that restates the task and its metadata so you know what goes.
type ConfirmModel struct {
	Open   bool
	Title  string
	Prompt string
	Task   *domain.Task
	Now    time.Time
	Width  int
	Height int
	Styles Styles
	Icons  Icons
}

func NewConfirm(styles Styles) ConfirmModel { return ConfirmModel{Styles: styles} }

func (c *ConfirmModel) OpenFor(title, prompt string) {
	c.Open = true
	c.Title = title
	c.Prompt = prompt
	c.Task = nil
}

// OpenTask opens a destructive confirmation for one task.
func (c *ConfirmModel) OpenTask(title string, task domain.Task, prompt string, now time.Time) {
	c.OpenFor(title, prompt)
	c.Task = &task
	c.Now = now
}

func (c *ConfirmModel) Close() { c.Open = false }

func (c *ConfirmModel) SetSize(width, height int) { c.Width, c.Height = width, height }

func (c *ConfirmModel) Update(msg tea.Msg) ConfirmAction {
	if !c.Open {
		return ConfirmNone
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return ConfirmNone
	}
	switch strings.ToLower(keyMsg.String()) {
	case "y":
		c.Close()
		return ConfirmYes
	case "n":
		c.Close()
		return ConfirmNo
	case "esc", "escape":
		c.Close()
		return ConfirmCancel
	default:
		return ConfirmNone
	}
}

func (c ConfirmModel) View() string {
	if !c.Open || c.Width <= 0 || c.Height <= 0 {
		return ""
	}
	icons := c.Icons.orUnicode()
	width := ModalWidth(ConfirmWidth, c.Width)
	content := FrameContentWidth(width)
	rows := []FrameRow{{}}
	yes, no := "yes", "no"
	if c.Task != nil {
		yes, no = "delete", "keep"
		now := c.Now
		if now.IsZero() {
			now = time.Now()
		}
		rows = append(rows, row(txt(c.Task.Description).bold()))
		var meta []Span
		for index, part := range taskMetadata(*c.Task, TaskRowOptions{Now: now}, icons) {
			if index > 0 {
				meta = append(meta, muted(" "+icons.Dot+" "))
			}
			meta = append(meta, part.spans...)
		}
		if len(meta) > 0 {
			rows = append(rows, row(meta...))
		}
		rows = append(rows, FrameRow{})
	}
	for _, line := range WrapText(c.Prompt, content) {
		rows = append(rows, row(txt(line)))
	}
	yesFill := FillAccent
	if c.Task != nil {
		yesFill = FillRed
	}
	rows = append(rows, FrameRow{})
	rows = append(rows, chipRows(content, []Span{chip("y", yesFill), txt(" " + yes)}, []Span{chip("n", FillSelection), muted(" " + no)})...)
	rows = append(rows, FrameRow{})
	frame := Frame{
		Title: c.Title, Danger: c.Task != nil, Rows: fitRows(rows, ModalMaxRows(c.Height)), Width: width, MaxRows: ModalMaxRows(c.Height),
		Keys: []Hint{{"esc", "cancel"}},
	}
	return strings.Join(c.Styles.RenderFrame(frame, icons), "\n")
}
