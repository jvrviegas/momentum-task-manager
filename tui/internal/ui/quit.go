package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// QuitChoice routes the explicit unsynced-change decision.
type QuitChoice string

const (
	QuitNone   QuitChoice = "none"
	QuitSync   QuitChoice = "sync"
	QuitLocal  QuitChoice = "local"
	QuitCancel QuitChoice = "cancel"
)

// QuitModel displays the only choices that can follow an unsynced quit.
type QuitModel struct {
	Open   bool
	Width  int
	Height int
	Styles Styles
	Icons  Icons
}

func NewQuit(styles Styles) QuitModel          { return QuitModel{Styles: styles} }
func (q *QuitModel) SetSize(width, height int) { q.Width, q.Height = width, height }
func (q *QuitModel) OpenQuit()                 { q.Open = true }
func (q *QuitModel) Close()                    { q.Open = false }

func (q *QuitModel) Update(msg tea.Msg) QuitChoice {
	if !q.Open {
		return QuitNone
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return QuitNone
	}
	switch strings.ToLower(keyMsg.String()) {
	case "s":
		q.Close()
		return QuitSync
	case "q":
		q.Close()
		return QuitLocal
	case "esc", "escape":
		q.Close()
		return QuitCancel
	default:
		return QuitNone
	}
}

func (q QuitModel) View() string {
	if !q.Open || q.Width <= 0 || q.Height <= 0 {
		return ""
	}
	icons := q.Icons.orUnicode()
	width := ModalWidth(QuitWidth, q.Width)
	rows := []FrameRow{{}, row(sp(icons.Local, ToneMedium), txt(" Unsynced changes"))}
	for _, line := range WrapText("Quitting without syncing keeps them in Taskwarrior for the next sync.", FrameContentWidth(width)) {
		rows = append(rows, row(muted(line)))
	}
	rows = append(rows, FrameRow{})
	rows = append(rows, chipRows(FrameContentWidth(width), []Span{chip("s", FillAccent), txt(" Sync and quit")}, []Span{chip("q", FillSelection), muted(" Quit without syncing")})...)
	rows = append(rows, FrameRow{})
	frame := Frame{Title: "Quit Momentum?", Rows: fitRows(rows, ModalMaxRows(q.Height)), Width: width, MaxRows: ModalMaxRows(q.Height), Keys: []Hint{{"esc", "Cancel"}}}
	return strings.Join(q.Styles.RenderFrame(frame, icons), "\n")
}

// MinimumSizeMessage explains why nothing renders and what is needed. Plain
// “x” keeps it ASCII-safe.
func MinimumSizeMessage(width, height int, styles Styles) string {
	if width < 1 || height < 1 {
		return ""
	}
	lines := []string{
		"",
		styles.Line(width, FillNone, txt(spaces(min(1, width-20))), sp("!", ToneRed).bold(), txt(" "), txt("Terminal too small").bold()),
		"",
		styles.Line(width, FillNone, gap(3), muted(fmt.Sprintf("%-8s", "Now")), txt(fmt.Sprintf("%d x %d", width, height))),
		styles.Line(width, FillNone, gap(3), muted(fmt.Sprintf("%-8s", "Needs")), txt(fmt.Sprintf("%d x %d", MinimumWidth, MinimumHeight)).bold()),
		"",
		styles.Line(width, FillNone, gap(3), muted("Resize, or "), txt("q").bold(), muted(" to quit")),
	}
	if len(lines) > height {
		lines = lines[1:]
	}
	for len(lines) > height {
		lines = append(lines[:len(lines)-2], lines[len(lines)-1])
	}
	return strings.Join(lines, "\n")
}
