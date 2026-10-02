package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Tone names a foreground role from the palette.
type Tone uint8

const (
	ToneText Tone = iota
	ToneMuted
	ToneAccent
	ToneCyan
	ToneRed
	ToneHigh
	ToneMedium
	ToneLow
	ToneBorder
	ToneSurface
	ToneDim
)

// Fill names a background role. Only Panel and Selection are used as row
// fills; Accent and Red back confirm chips.
type Fill uint8

const (
	FillNone Fill = iota
	FillPanel
	FillSelection
	FillAccent
	FillRed
)

// Span is one run of same-styled cells. A Grow span repeats its first cell
// to absorb the remaining width of a line; several Grow spans share it.
type Span struct {
	Text      string
	Tone      Tone
	Bold      bool
	Underline bool
	Cursor    bool
	Bg        Fill
	Grow      bool
}

func sp(text string, tone Tone) Span { return Span{Text: text, Tone: tone} }
func txt(text string) Span           { return Span{Text: text} }
func muted(text string) Span         { return Span{Text: text, Tone: ToneMuted} }
func gap(cells int) Span             { return Span{Text: spaces(cells)} }
func grow() Span                     { return Span{Text: " ", Grow: true} }
func rule(glyph string) Span         { return Span{Text: glyph, Tone: ToneBorder, Grow: true} }

func (s Span) bold() Span      { s.Bold = true; return s }
func (s Span) underline() Span { s.Underline = true; return s }
func (s Span) on(f Fill) Span  { s.Bg = f; return s }

func spansWidth(spans []Span) int {
	total := 0
	for _, span := range spans {
		if !span.Grow {
			total += lipgloss.Width(span.Text)
		}
	}
	return total
}

// fitSpans lays spans into exactly width cells: Grow spans share the slack
// and an overflowing line truncates with … in the tone of its last kept cell.
func fitSpans(spans []Span, width int) []Span {
	if width <= 0 {
		return nil
	}
	fixed := spansWidth(spans)
	if fixed > width {
		out := make([]Span, 0, len(spans)+1)
		budget := width - 1
		last := Span{}
		for _, span := range spans {
			if span.Grow || budget <= 0 {
				continue
			}
			w := lipgloss.Width(span.Text)
			if w <= budget {
				out = append(out, span)
				budget -= w
				last = span
				continue
			}
			span.Text = truncateCells(span.Text, budget)
			out = append(out, span)
			budget = 0
			last = span
		}
		last.Text, last.Cursor = "…", false
		out = append(out, last)
		return out
	}
	growers := 0
	for _, span := range spans {
		if span.Grow {
			growers++
		}
	}
	slack := width - fixed
	out := make([]Span, 0, len(spans)+1)
	seen := 0
	for _, span := range spans {
		if !span.Grow {
			out = append(out, span)
			continue
		}
		n := slack / growers
		if seen < slack%growers {
			n++
		}
		seen++
		if n <= 0 {
			continue
		}
		cell := []rune(span.Text)
		if len(cell) == 0 {
			cell = []rune{' '}
		}
		span.Text = strings.Repeat(string(cell[0]), n)
		span.Grow = false
		out = append(out, span)
	}
	if growers == 0 && slack > 0 {
		out = append(out, gap(slack))
	}
	return out
}

func truncateCells(value string, width int) string {
	result := ""
	for _, r := range value {
		next := result + string(r)
		if lipgloss.Width(next) > width {
			break
		}
		result = next
	}
	return result
}

// Line paints spans into exactly width cells over an optional row fill.
// Muted text on a fill switches to MutedFill so it keeps AA contrast.
func (s Styles) Line(width int, bg Fill, spans ...Span) string {
	var builder strings.Builder
	for _, span := range fitSpans(spans, width) {
		builder.WriteString(s.paint(span, bg))
	}
	return builder.String()
}

func (s Styles) paint(span Span, rowBg Fill) string {
	if span.Text == "" {
		return ""
	}
	bg := span.Bg
	if bg == FillNone {
		bg = rowBg
	}
	style := lipgloss.NewStyle().Foreground(s.tone(span.Tone, bg != FillNone))
	if bg != FillNone {
		style = style.Background(s.fill(bg))
	}
	if span.Cursor {
		style = lipgloss.NewStyle().Foreground(s.tone(ToneSurface, false)).Background(s.tone(ToneText, false))
	}
	if span.Bold {
		style = style.Bold(true)
	}
	if span.Underline {
		style = style.Underline(true)
	}
	return style.Render(span.Text)
}

// Hint is one key and its label, e.g. {"c", "capture"}.
type Hint struct {
	Key   string
	Label string
}

// hintChips renders “ c  capture” chips: bold Text key on Panel, Muted
// label, 2 cells apart.
func hintChips(hints []Hint) []Span {
	spans := make([]Span, 0, len(hints)*3)
	for index, hint := range hints {
		if index > 0 {
			spans = append(spans, gap(2))
		}
		spans = append(spans, txt(" "+hint.Key+" ").bold().on(FillPanel), muted(" "+hint.Label))
	}
	return spans
}

// fitHints keeps as many leading hints as fit in width; hints drop from the
// right so the most common keys survive.
func fitHints(hints []Hint, width int) []Span {
	for count := len(hints); count > 0; count-- {
		spans := hintChips(hints[:count])
		if spansWidth(spans) <= width {
			return spans
		}
	}
	return nil
}

// KeyHints renders a footer hints row: hints on the left, extra on the right.
func KeyHints(hints, extra []Hint, width int, styles Styles) string {
	if width <= 0 {
		return ""
	}
	right := hintChips(extra)
	if len(extra) > 0 && spansWidth(right)+2 > width-spansWidth(hintChips(hints)) {
		right = nil
	}
	left := fitHints(hints, width-spansWidth(right))
	return styles.Line(width, FillNone, append(append(left, grow()), right...)...)
}

// chip renders a confirm chip such as “ y ” on Red with Surface text.
func chip(key string, fill Fill) Span {
	tone := ToneSurface
	if fill == FillSelection || fill == FillPanel {
		tone = ToneText
	}
	return Span{Text: " " + key + " ", Tone: tone, Bold: true, Bg: fill}
}

// inputSpans renders a single-line value with a reverse-video cursor cell,
// scrolling horizontally so the cursor stays visible. cell styles rune i;
// nil means plain Text. An empty value shows a Muted placeholder.
func inputSpans(value string, cursor, width int, focused bool, placeholder string, cell func(index int, r rune) Span) []Span {
	if width <= 0 {
		return nil
	}
	runes := []rune(value)
	if cell == nil {
		cell = func(_ int, r rune) Span { return txt(string(r)) }
	}
	cursor = min(max(0, cursor), len(runes))
	if len(runes) == 0 {
		spans := make([]Span, 0, 2)
		if focused {
			spans = append(spans, Span{Text: " ", Cursor: true})
		}
		if placeholder != "" {
			spans = append(spans, muted(Truncate(placeholder, width-spansWidth(spans))))
		}
		return spans
	}
	cursorCells := 0
	if focused && cursor == len(runes) {
		cursorCells = 1
	}
	start := 0
	used := cursorCells
	for index := cursor - 1; index >= 0; index-- {
		w := lipgloss.Width(string(runes[index]))
		if used+w > width {
			start = index + 1
			break
		}
		used += w
	}
	spans := make([]Span, 0, len(runes)-start+1)
	used = 0
	for index := start; index < len(runes); index++ {
		w := lipgloss.Width(string(runes[index]))
		if used+w > width-cursorCells {
			break
		}
		span := cell(index, runes[index])
		if focused && index == cursor {
			span.Cursor = true
		}
		spans = append(spans, span)
		used += w
	}
	if cursorCells > 0 {
		spans = append(spans, Span{Text: " ", Cursor: true})
	}
	return spans
}
