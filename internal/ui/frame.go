package ui

import (
	"fmt"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Modal widths from the component specs. Every modal is min(spec, W − 2 ×
// margin) wide, with a 2-cell screen margin (1 below NarrowBreakpoint).
const (
	QuickAddWidth = 78
	DetailsWidth  = 84
	EditWidth     = 80
	HelpWidth     = 86
	ConfirmWidth  = 64
	QuitWidth     = 60
	SettingsWidth = 72
	RenameWidth   = 86
	PlannerWidth  = 86

	// frameInset is border + pad + gutter + space on the left and pad +
	// border on the right: content is w − 6 cells wide and starts 4 in.
	frameInset = 6
)

// FrameRow is one content row: an optional gutter mark (▌, •, !) and an
// optional row fill that runs from pad to pad.
type FrameRow struct {
	Spans    []Span
	Mark     string
	MarkTone Tone
	Bg       Fill
}

func row(spans ...Span) FrameRow { return FrameRow{Spans: spans} }

// Frame is the shared modal shell: title and context in the top border, keys
// and position in the bottom border, a Panel fill on every cell.
type Frame struct {
	Title    string
	Context  []Span
	Keys     []Hint
	Position []Span
	Danger   bool
	Rows     []FrameRow
	Offset   int
	Width    int
	MaxRows  int
}

// ModalWidth applies the screen margin to a spec width.
func ModalWidth(spec, terminalWidth int) int {
	margin := 2
	if terminalWidth < NarrowBreakpoint {
		margin = 1
	}
	return max(1, min(spec, terminalWidth-2*margin))
}

// ModalMaxRows is the content-row budget: one blank screen row above and
// below the frame, plus its two borders.
func ModalMaxRows(terminalHeight int) int {
	return max(1, terminalHeight-4)
}

// FrameContentWidth is the width of a frame's content column.
func FrameContentWidth(frameWidth int) int {
	return max(1, frameWidth-frameInset)
}

// MaxOffset returns the furthest scroll offset for rows in a viewport.
func MaxOffset(rows, viewport int) int {
	return max(0, rows-max(1, viewport))
}

// RenderFrame draws the frame as exactly Width-wide lines.
func (s Styles) RenderFrame(f Frame, icons Icons) []string {
	icons = icons.orUnicode()
	w := f.Width
	if w < frameInset+1 {
		w = frameInset + 1
	}
	maxRows := f.MaxRows
	if maxRows <= 0 {
		maxRows = len(f.Rows)
	}
	rows := f.Rows
	if len(rows) == 0 {
		rows = []FrameRow{{}}
	}
	total := len(rows)
	offset := 0
	var thumb [2]int
	scrolling := total > maxRows
	position := f.Position
	if scrolling {
		offset = min(max(0, f.Offset), total-maxRows)
		rows = rows[offset : offset+maxRows]
		size := max(1, int(math.Round(float64(maxRows*maxRows)/float64(total))))
		top := min(maxRows-size, int(math.Round(float64(offset*maxRows)/float64(total))))
		thumb = [2]int{top, top + size - 1}
		if position == nil {
			position = []Span{muted(fmt.Sprintf("%d-%d of %d", offset+1, offset+len(rows), total))}
		}
	}

	borderTone := ToneBorder
	if f.Danger {
		borderTone = ToneRed
	}
	border := func(text string) Span { return sp(text, borderTone) }

	// Border budgets: corner, "─ " label " ", optional " " extra " ─", corner.
	top := []Span{border(icons.CornerTL)}
	if f.Title != "" {
		label := f.Title
		if f.Danger {
			label = icons.Error + " " + label
		}
		title := txt(Truncate(label, max(1, w-5))).bold()
		if f.Danger {
			title.Tone = ToneRed
		}
		top = append(top, border(icons.Rule+" "), title, border(" "))
	}
	top = append(top, rule(icons.Rule))
	top[len(top)-1].Tone = borderTone
	if len(f.Context) > 0 && spansWidth(top)+spansWidth(f.Context)+4 <= w {
		top = append(top, border(" "))
		top = append(top, f.Context...)
		top = append(top, border(" "+icons.Rule))
	}
	top = append(top, border(icons.CornerTR))

	lines := make([]string, 0, len(rows)+2)
	lines = append(lines, s.Line(w, FillPanel, top...))
	contentWidth := w - frameInset
	for index, r := range rows {
		bg := r.Bg
		if bg == FillNone {
			bg = FillPanel
		}
		mark := " "
		if r.Mark != "" {
			mark = r.Mark
		}
		markTone := r.MarkTone
		if r.Mark != "" && markTone == ToneText {
			markTone = ToneAccent
		}
		inner := []Span{txt(" "), sp(mark, markTone), txt(" ")}
		inner = append(inner, fitSpans(r.Spans, contentWidth)...)
		inner = append(inner, txt(" "))
		right := border(icons.Divider)
		if scrolling && index >= thumb[0] && index <= thumb[1] {
			right = muted(icons.Thumb)
		}
		lines = append(lines, s.Line(1, FillPanel, border(icons.Divider))+s.Line(w-2, bg, inner...)+s.Line(1, FillPanel, right))
	}

	bottom := []Span{border(icons.CornerBL)}
	keyBudget := w - 5
	if len(position) > 0 {
		keyBudget -= spansWidth(position) + 3
	}
	if keys := frameKeys(f.Keys, keyBudget); len(keys) > 0 {
		bottom = append(bottom, border(icons.Rule+" "))
		bottom = append(bottom, keys...)
		bottom = append(bottom, border(" "))
	}
	bottom = append(bottom, rule(icons.Rule))
	bottom[len(bottom)-1].Tone = borderTone
	if len(position) > 0 && spansWidth(bottom)+spansWidth(position)+4 <= w {
		bottom = append(bottom, border(" "))
		bottom = append(bottom, position...)
		bottom = append(bottom, border(" "+icons.Rule))
	}
	bottom = append(bottom, border(icons.CornerBR))
	lines = append(lines, s.Line(w, FillPanel, bottom...))
	return lines
}

func frameKeys(hints []Hint, width int) []Span {
	for count := len(hints); count > 0; count-- {
		spans := make([]Span, 0, count*3)
		for index, hint := range hints[:count] {
			if index > 0 {
				spans = append(spans, gap(3))
			}
			spans = append(spans, txt(hint.Key).bold(), muted(" "+hint.Label))
		}
		if spansWidth(spans) <= width {
			return spans
		}
	}
	return nil
}

// Placement is where a frame sits on the terminal grid.
type Placement struct {
	X, Y int
}

// CenterPlacement centres a frame; quick capture overrides Y.
func CenterPlacement(frameWidth, frameHeight, width, height int) Placement {
	return Placement{X: max(0, (width-frameWidth)/2), Y: max(0, (height-frameHeight)/2)}
}

// Compose draws frame lines over base. The base is redrawn in Dim with its
// fills removed, which is how a terminal shows an inactive backdrop.
func (s Styles) Compose(base string, frame []string, at Placement, width, height int) string {
	baseLines := strings.Split(base, "\n")
	dim := lipgloss.NewStyle().Foreground(s.tone(ToneDim, false))
	paintDim := func(text string) string {
		if strings.TrimSpace(text) == "" {
			return text
		}
		return dim.Render(text)
	}
	frameWidth := 0
	if len(frame) > 0 {
		frameWidth = lipgloss.Width(frame[0])
	}
	lines := make([]string, height)
	for y := 0; y < height; y++ {
		plain := ""
		if y < len(baseLines) {
			plain = ansi.Strip(baseLines[y])
		}
		plain = PadRight(Truncate(plain, width), width)
		index := y - at.Y
		if index < 0 || index >= len(frame) {
			lines[y] = paintDim(plain)
			continue
		}
		left := ansi.Cut(plain, 0, at.X)
		right := ansi.Cut(plain, at.X+frameWidth, width)
		lines[y] = paintDim(left) + frame[index] + paintDim(right)
	}
	return strings.Join(lines, "\n")
}

// chipRows lays confirm chips on one row when they fit and one per row
// otherwise, so a choice is never truncated.
func chipRows(width int, choices ...[]Span) []FrameRow {
	joined := make([]Span, 0, len(choices)*3)
	for index, choice := range choices {
		if index > 0 {
			joined = append(joined, gap(5))
		}
		joined = append(joined, choice...)
	}
	if spansWidth(joined) <= width {
		return []FrameRow{row(joined...)}
	}
	rows := make([]FrameRow, 0, len(choices))
	for _, choice := range choices {
		rows = append(rows, row(choice...))
	}
	return rows
}

// fitRows drops blank rows, then the rows just under the first, until the
// frame fits: short terminals keep the subject and the choices at the end.
func fitRows(rows []FrameRow, maxRows int) []FrameRow {
	rows = append([]FrameRow(nil), rows...)
	for len(rows) > maxRows {
		removed := false
		for index := len(rows) - 1; index >= 0; index-- {
			if len(rows[index].Spans) == 0 && rows[index].Mark == "" {
				rows = append(rows[:index], rows[index+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			if len(rows) < 3 {
				return rows[len(rows)-maxRows:]
			}
			rows = append(rows[:1], rows[2:]...)
		}
	}
	return rows
}
