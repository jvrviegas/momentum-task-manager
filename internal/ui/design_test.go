package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func designStyles() Styles { return NewStyles(ResolveTheme("dark", true)) }

func TestFrameDrawsTitleContextKeysAtExactWidth(t *testing.T) {
	lines := designStyles().RenderFrame(Frame{
		Title: "Task details", Context: []Span{muted("read-only")}, Width: 40, MaxRows: 10,
		Keys: []Hint{{"↑↓", "scroll"}, {"esc", "close"}},
		Rows: []FrameRow{{}, row(txt("Body")), {Spans: []Span{txt("Picked")}, Mark: "▌", Bg: FillSelection}},
	}, IconsFor("unicode"))
	if len(lines) != 5 {
		t.Fatalf("lines=%d", len(lines))
	}
	for _, line := range lines {
		if lipgloss.Width(line) != 40 {
			t.Fatalf("width=%d line=%q", lipgloss.Width(line), line)
		}
	}
	plain := strings.Split(sanitizeComponentRender(strings.Join(lines, "\n")), "\n")
	if !strings.HasPrefix(plain[0], "╭─ Task details ") || !strings.HasSuffix(plain[0], " read-only ─╮") {
		t.Fatalf("top border=%q", plain[0])
	}
	// Content starts 4 cells in: border, pad, gutter, space.
	if !strings.HasPrefix(plain[2], "│   Body") || !strings.HasPrefix(plain[3], "│ ▌ Picked") {
		t.Fatalf("rows=%q", plain[1:4])
	}
	if !strings.HasPrefix(plain[4], "╰─ ↑↓ scroll   esc close ") || !strings.HasSuffix(plain[4], "╯") {
		t.Fatalf("bottom border=%q", plain[4])
	}
}

func TestFrameScrollShowsThumbAndPosition(t *testing.T) {
	rows := make([]FrameRow, 10)
	for index := range rows {
		rows[index] = row(txt(string(rune('a' + index))))
	}
	plain := sanitizeComponentRender(strings.Join(designStyles().RenderFrame(Frame{Title: "List", Rows: rows, Offset: 4, Width: 30, MaxRows: 4}, Icons{}), "\n"))
	if !strings.Contains(plain, "5-8 of 10") || !strings.Contains(plain, "┃") || strings.Contains(plain, "│   a") || !strings.Contains(plain, "│   e") {
		t.Fatalf("scrolled frame=%s", plain)
	}
}

func TestDangerFrameMarksTitleWithoutColor(t *testing.T) {
	plain := sanitizeComponentRender(strings.Join(designStyles().RenderFrame(Frame{Title: "Delete task", Danger: true, Width: 30}, Icons{}), "\n"))
	if !strings.HasPrefix(plain, "╭─ ! Delete task ") {
		t.Fatalf("danger frame=%s", plain)
	}
}

func TestFrameBordersFitNarrowWidths(t *testing.T) {
	for _, width := range []int{12, 20, 26} {
		lines := designStyles().RenderFrame(Frame{
			Title: "Quick capture", Context: []Span{muted("ctrl+k")}, Width: width, MaxRows: 2,
			Keys: []Hint{{"↑↓", "select"}, {"tab", "accept"}, {"esc", "close"}},
			Rows: []FrameRow{row(txt("a")), row(txt("b")), row(txt("c"))},
		}, Icons{})
		for _, line := range lines {
			if lipgloss.Width(line) != width {
				t.Fatalf("width=%d line=%q", width, sanitizeComponentRender(line))
			}
		}
		last := sanitizeComponentRender(lines[len(lines)-1])
		if !strings.HasSuffix(last, "╯") {
			t.Fatalf("width=%d lost the corner: %q", width, last)
		}
	}
}

func TestComposeDimsBaseAndPlacesFrame(t *testing.T) {
	styles := designStyles()
	base := strings.Join([]string{styles.Line(10, FillSelection, txt("abcdefghij")), "klmnopqrst", "uvwxyz"}, "\n")
	out := styles.Compose(base, []string{"[XY]"}, Placement{X: 3, Y: 1}, 10, 3)
	lines := strings.Split(sanitizeComponentRender(out), "\n")
	if lines[0] != "abcdefghij" || lines[1] != "klm[XY]rst" || lines[2] != "uvwxyz" {
		t.Fatalf("composed=%q", lines)
	}
	// The backdrop loses its fills: no background escape survives on row 0.
	if first := strings.Split(out, "\n")[0]; strings.Contains(first, "48;") {
		t.Fatalf("backdrop kept a fill: %q", first)
	}
}

func TestRowMetadataDropsLowestPriorityFirst(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	due := now.AddDate(0, 0, -3)
	start := now.Add(-time.Hour)
	task := domain.Task{Description: "Renew the certificate", Status: "pending", Project: "ops.infrastructure", Due: &due, Start: &start}
	render := func(width int) string {
		return sanitizeComponentRender(RenderTaskRow(task, TaskRowOptions{Width: width, ShowMetadata: true, Density: DensityComfortable, Now: now, Styles: designStyles(), Icons: IconsFor("unicode")}))
	}
	wide := render(90)
	if !strings.Contains(wide, "Active · #ops.infrastructure · ! Overdue · Due Sep 5") {
		t.Fatalf("wide=%q", wide)
	}
	// Project (2) goes before Active (3); the overdue phrase (5) stays last.
	medium := render(44)
	if strings.Contains(medium, "#ops") || !strings.Contains(medium, "Active · ! Overdue") {
		t.Fatalf("medium=%q", medium)
	}
	tight := render(34)
	if strings.Contains(tight, "Active") || !strings.Contains(tight, "! Overdue · Due Sep 5") {
		t.Fatalf("tight=%q", tight)
	}
}

func TestCompletedRowsUseTheSlotForCompletionTime(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	today := now.Add(-time.Hour)
	earlier := now.AddDate(0, 0, -5)
	options := TaskRowOptions{Width: 50, ShowMetadata: true, Completed: true, Density: DensityComfortable, Now: now, Styles: designStyles(), Icons: IconsFor("unicode")}
	first := strings.Split(sanitizeComponentRender(RenderTaskRow(domain.Task{Description: "Done", Status: "completed", End: &today, Priority: "H"}, options)), "\n")[0]
	if !strings.HasSuffix(first, "09:00") || strings.Contains(first, "High") || !strings.HasPrefix(first, "  ✓ Done") {
		t.Fatalf("today completed row=%q", first)
	}
	if row := sanitizeComponentRender(RenderTaskRow(domain.Task{Description: "Old", Status: "completed", End: &earlier}, options)); !strings.HasSuffix(row, "Sep 3 10:00") {
		t.Fatalf("earlier completed row=%q", row)
	}
}

func TestFriendlyDatesNameYesterdayAndOmitMidnight(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	for value, want := range map[time.Time]string{
		time.Date(2026, 9, 7, 18, 0, 0, 0, time.UTC): "Yesterday 18:00",
		time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC):  "Tomorrow",
		time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC): "Today 17:00",
		time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC): "Oct 3",
	} {
		if got := formatDate(value, now); got != want {
			t.Errorf("formatDate(%s)=%q want %q", value, got, want)
		}
	}
}

func TestTabUnderlineSitsUnderExactlyTheActiveTab(t *testing.T) {
	items := []NavItem{
		{Key: "inbox", Label: "Inbox", Count: 11, Icon: "▱"},
		{Key: "today", Label: "Today", Count: 8, Icon: "◷"},
		{Key: "completed", Label: "Completed", Icon: "✓", HideCount: true},
	}
	for _, width := range []int{79, 49, 30} {
		lines := strings.Split(sanitizeComponentRender(RenderTabs("today", items, width, designStyles(), Icons{})), "\n")
		if len(lines) != 2 {
			t.Fatalf("width=%d tabs=%q", width, lines)
		}
		top, under := []rune(lines[0]), []rune(lines[1])
		start := strings.Index(lines[0], "◷")
		if start < 0 {
			t.Fatalf("width=%d active tab missing: %q", width, lines[0])
		}
		col := len([]rune(lines[0][:start]))
		if under[col] != '━' || (col > 1 && under[col-1] != '─') {
			t.Fatalf("width=%d underline=%q tabs=%q", width, lines[1], lines[0])
		}
		if width < MinimalTabsBreakpoint && !strings.Contains(string(top), "·•·") {
			t.Fatalf("width=%d expected position dots: %q", width, lines[0])
		}
	}
}

func TestSectionHeaderKeepsCountWhenNarrow(t *testing.T) {
	plain := sanitizeComponentRender(RenderSectionHeader("Scheduled Today", 12, ToneText, 16, designStyles(), Icons{}))
	if !strings.HasSuffix(plain, "  12") || !strings.HasPrefix(plain, "Schedul") {
		t.Fatalf("header=%q", plain)
	}
	if wide := sanitizeComponentRender(RenderSectionHeader("Overdue", 2, ToneRed, 30, designStyles(), Icons{})); !strings.HasPrefix(wide, "Overdue  2  ───") {
		t.Fatalf("wide header=%q", wide)
	}
}

func TestSearchHighlightSplitsMatchedText(t *testing.T) {
	spans := highlightTitle("Ask Acme procurement", "acme", ToneText, false)
	if len(spans) != 3 || spans[1].Text != "Acme" || !spans[1].Underline || !spans[1].Bold || spans[1].Tone != ToneAccent {
		t.Fatalf("spans=%#v", spans)
	}
	if spans := highlightTitle("Nothing here", "project:work", ToneText, false); len(spans) != 1 {
		t.Fatalf("qualifier-only query highlighted: %#v", spans)
	}
}

func TestCaptureInputUnderlinesTriggerTokens(t *testing.T) {
	cell := captureTokenCell(`Call #work !high \#tag`)
	if span := cell(5, '#'); span.Tone != ToneCyan || !span.Underline {
		t.Fatalf("project token=%#v", span)
	}
	if span := cell(11, '!'); span.Tone != ToneHigh || !span.Underline {
		t.Fatalf("priority token=%#v", span)
	}
	if span := cell(0, 'C'); span.Underline {
		t.Fatalf("plain text underlined: %#v", span)
	}
	if span := cell(17, '\\'); span.Underline {
		t.Fatalf("escaped trigger underlined: %#v", span)
	}
}
