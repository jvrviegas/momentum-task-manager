package ui

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// WideBreakpoint is the first width at which a sidebar leaves the main
	// content with a comfortable reading width. Keep this value centralized:
	// changing it in one place must not change mouse or row geometry.
	WideBreakpoint = 104
	// NarrowBreakpoint is the first width at which comfortable two-line task
	// blocks fit. Widths below it use compact single-line rows.
	NarrowBreakpoint = 50
	// Minimum dimensions keep the warning itself usable.
	MinimumWidth  = 28
	MinimumHeight = 8

	// SidebarWidth is the target width of the wide navigation rail.
	SidebarWidth = 24
	// SidebarTargetWidth is an explicit name for callers describing layout
	// policy rather than rendering a sidebar.
	SidebarTargetWidth = SidebarWidth

	// The UI uses a deliberately small spacing vocabulary. Gaps are terminal
	// cells, not styling metadata, so they are included in geometry budgets.
	LocalGap      = 1
	SectionGap    = 2
	ContentGutter = 2
	ModalEdge     = 2

	// Modal maximums keep forms and read-only content readable on very wide
	// terminals while still allowing the terminal to determine small sizes.
	ModalMaxWidth    = 78
	DetailsMaxWidth  = 86
	HelpMaxWidth     = 86
	QuickAddMaxWidth = 78
)

// RowDensity identifies the two supported task-list presentations.
type RowDensity string

const (
	DensityComfortable RowDensity = "comfortable"
	DensityCompact     RowDensity = "compact"
)

// LayoutMode identifies the responsive composition.
type LayoutMode string

const (
	LayoutMinimum LayoutMode = "minimum"
	LayoutTabs    LayoutMode = "tabs"
	LayoutWide    LayoutMode = "wide"
)

// Layout contains centralized responsive decisions. ContentWidth retains the
// historical one-cell allocation for compatibility with embedders; MainWidth
// is the actual width passed to the main renderer after the approved
// two-column gutter is budgeted.
type Layout struct {
	Mode            LayoutMode
	Width           int
	Height          int
	SidebarWidth    int
	ContentWidth    int
	MainWidth       int
	ContentGutter   int
	ShowSidebar     bool
	ShowTabs        bool
	ShowRowMetadata bool
	RowDensity      RowDensity
	Usable          bool
}

// ShellGeometry is the shared vertical contract for rendering and mouse hit
// testing. BodyTop is an absolute terminal row; BodyHeight is the exact line
// budget available to task/settings content. Optional spacing collapses before
// meaningful content is removed.
type ShellGeometry struct {
	TerminalHeight int
	HeaderTop      int
	HeaderHeight   int
	NavTop         int
	NavHeight      int
	ContentGap     int
	SearchTop      int
	SearchHeight   int
	BodyTop        int
	BodyHeight     int
	FooterGap      int
	FooterTop      int
	FooterHeight   int
}

// Geometry derives a bounded shell from terminal dimensions. Search occupies
// a real header/content row rather than being appended after an already-fit
// base view, which prevents the search overlay from clipping the top of the
// application.
func (l Layout) Geometry(searchOpen bool) ShellGeometry {
	if l.Height <= 0 {
		return ShellGeometry{}
	}

	geometry := ShellGeometry{
		TerminalHeight: l.Height,
		HeaderTop:      0,
		HeaderHeight:   1,
		NavHeight:      0,
		ContentGap:     0,
		SearchHeight:   0,
		FooterGap:      0,
		FooterHeight:   1,
	}
	if l.ShowTabs {
		geometry.NavHeight = 1
	}
	// A two-line footer and local breathing room are the comfortable default.
	// Short terminals first lose gaps, then the optional footer line.
	if l.Height >= 12 {
		geometry.ContentGap = LocalGap
		geometry.FooterGap = LocalGap
		geometry.FooterHeight = 2
	}
	if searchOpen {
		geometry.SearchHeight = 1
	}

	geometry.recalculate()
	if geometry.BodyHeight < 1 {
		geometry.FooterGap = 0
		geometry.recalculate()
	}
	if geometry.BodyHeight < 1 && geometry.ContentGap > 0 {
		geometry.ContentGap = 0
		geometry.recalculate()
	}
	if geometry.BodyHeight < 1 && geometry.FooterHeight > 1 {
		geometry.FooterHeight = 1
		geometry.recalculate()
	}
	if geometry.BodyHeight < 1 {
		geometry.BodyHeight = 1
		geometry.FooterTop = geometry.BodyTop + geometry.BodyHeight + geometry.FooterGap
	}
	return geometry
}

func (g *ShellGeometry) recalculate() {
	g.NavTop = g.HeaderTop + g.HeaderHeight
	g.SearchTop = g.NavTop + g.NavHeight + g.ContentGap
	g.BodyTop = g.SearchTop + g.SearchHeight
	g.BodyHeight = gFooterBodyHeight(g)
	g.FooterTop = g.BodyTop + g.BodyHeight + g.FooterGap
}

func gFooterBodyHeight(g *ShellGeometry) int {
	return g.TerminalHeight - g.BodyTop - g.FooterGap - g.FooterHeight
}

// ModalContentWidth returns the width available inside a bordered modal. The
// result is always positive for a usable terminal and never exceeds maximum.
func ModalContentWidth(terminalWidth, maximum int) int {
	// Two edge cells plus the two border cells are budgeted here. Lip Gloss
	// receives content width + 2 in the shared shell, leaving the requested
	// safe visual edge after centering even when the modal reaches its maximum.
	available := terminalWidth - (ModalEdge * 2) - 2
	if available < 1 {
		available = 1
	}
	if maximum > 0 && available > maximum {
		available = maximum
	}
	return available
}

// ModalContentHeight returns the height available to a modal's content after
// reserving a safe edge margin. It is intentionally conservative: the caller
// still reserves its own title/action rows.
func ModalContentHeight(terminalHeight, maximum int) int {
	available := terminalHeight - (ModalEdge * 2)
	if available < 1 {
		available = 1
	}
	if maximum > 0 && available > maximum {
		available = maximum
	}
	return available
}

// ChooseLayout maps terminal dimensions to a width-safe composition.
func ChooseLayout(width, height int) Layout {
	layout := Layout{
		Width:         width,
		Height:        height,
		ContentGutter: ContentGutter,
		Usable:        width >= MinimumWidth && height >= MinimumHeight,
	}
	if !layout.Usable {
		layout.Mode = LayoutMinimum
		return layout
	}

	layout.ShowRowMetadata = width >= NarrowBreakpoint
	if layout.ShowRowMetadata {
		layout.RowDensity = DensityComfortable
	} else {
		layout.RowDensity = DensityCompact
	}

	switch {
	case width >= WideBreakpoint:
		layout.Mode = LayoutWide
		layout.ShowSidebar = true
		layout.SidebarWidth = SidebarWidth
		// ContentWidth preserves the pre-readability one-cell allocation used
		// by existing embedders. MainWidth is the actual two-cell-gutter area.
		layout.ContentWidth = width - SidebarWidth - 1
		layout.MainWidth = width - SidebarWidth - ContentGutter
	case width >= NarrowBreakpoint:
		layout.Mode = LayoutTabs
		layout.ShowTabs = true
		layout.ContentWidth = width
		layout.MainWidth = width
	default:
		layout.Mode = LayoutTabs
		layout.ShowTabs = true
		layout.ContentWidth = width
		layout.MainWidth = width
	}
	if layout.ContentWidth < 1 {
		layout.ContentWidth = 1
	}
	if layout.MainWidth < 1 {
		layout.MainWidth = 1
	}
	return layout
}

// Truncate keeps a string within a terminal display width without splitting a
// UTF-8 rune. It does not count ANSI escape sequences because Lip Gloss does
// not count them either.
func Truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "…")
}

// PadRight adds display-width spaces, never byte-count spaces.
func PadRight(value string, width int) string {
	remaining := width - lipgloss.Width(value)
	if remaining <= 0 {
		return value
	}
	return value + spaces(remaining)
}

// WrapText wraps plain or ANSI-free content to display width. Long words are
// split by rune so a URL, UUID, or raw Taskwarrior value cannot make a modal
// wider than its terminal. Empty input returns one empty line for predictable
// field rendering.
func WrapText(value string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	paragraphs := strings.Split(value, "\n")
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := ""
		for _, word := range words {
			if line == "" {
				if lipgloss.Width(word) <= width {
					line = word
					continue
				}
				chunks := splitDisplayWidth(word, width)
				lines = append(lines, chunks[:len(chunks)-1]...)
				line = chunks[len(chunks)-1]
				continue
			}
			candidate := line + " " + word
			if lipgloss.Width(candidate) <= width {
				line = candidate
				continue
			}
			lines = append(lines, line)
			if lipgloss.Width(word) <= width {
				line = word
				continue
			}
			chunks := splitDisplayWidth(word, width)
			lines = append(lines, chunks[:len(chunks)-1]...)
			line = chunks[len(chunks)-1]
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func splitDisplayWidth(value string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	chunks := make([]string, 0, (utf8.RuneCountInString(value)/width)+1)
	chunk := ""
	for _, r := range value {
		candidate := chunk + string(r)
		if lipgloss.Width(candidate) > width {
			if chunk == "" {
				// A single wide rune cannot fit a one-cell line. Keep the
				// width contract with the shared ellipsis rather than letting
				// CJK/emoji glyphs overflow a modal.
				chunks = append(chunks, Truncate(string(r), width))
				continue
			}
			chunks = append(chunks, chunk)
			chunk = string(r)
			continue
		}
		// A zero-width combining rune belongs to the current chunk.
		chunk = candidate
	}
	if chunk == "" {
		chunks = append(chunks, "")
	} else {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func spaces(count int) string {
	if count <= 0 {
		return ""
	}
	result := make([]byte, count)
	for index := range result {
		result[index] = ' '
	}
	return string(result)
}
