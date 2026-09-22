package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestReviewLongFieldEditingKeepsCursorViewportVisible(t *testing.T) {
	for _, size := range [][2]int{{28, 8}, {49, 18}, {79, 24}, {120, 30}, {120, 8}} {
		for _, field := range []int{quickAddReviewDescription, quickAddReviewProject, quickAddReviewScheduled, quickAddReviewTags, quickAddReviewDue} {
			for kind, value := range map[string]string{"ascii": strings.Repeat("long-value.", 8), "unicode": strings.Repeat("界é作業.", 15)} {
				t.Run(fmt.Sprintf("%dx%d/%s/%s", size[0], size[1], quickAddReviewFieldNames[field], kind), func(t *testing.T) {
					q := reviewedQuickAdd(t, "Call tomorrow #work +client >today")
					q.ReviewInputs[field].SetValue(value)
					q.SetSize(size[0], size[1])
					q.focusReviewField(field)
					for _, key := range []rune{tea.KeyEnd, tea.KeyHome} {
						q.Update(tea.KeyPressMsg(tea.Key{Code: key}))
						q.Update(tea.KeyPressMsg(tea.Key{Code: 'Z', Text: "Z"}))
						view := q.View()
						// Include the styled cursor cell, not just a prefix of the text.
						if !strings.Contains(view, q.ReviewInputs[field].View()) {
							t.Fatalf("input viewport/cursor was truncated by the row: %q", view)
						}
						plain := ansi.Strip(view)
						var row string
						for _, line := range strings.Split(plain, "\n") {
							if strings.HasPrefix(line, quickAddReviewFieldNames[field]) {
								row = line
								break
							}
						}
						if !strings.Contains(row, "Z") {
							t.Fatalf("typed character/cursor viewport hidden: value=%q row=%q view=%q", q.ReviewInputs[field].Value(), row, view)
						}
						if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
							t.Fatalf("view exceeds terminal: %q", view)
						}
						q.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
						if q.ReviewInputs[field].Value() != value {
							t.Fatalf("delete did not restore value: %q", q.ReviewInputs[field].Value())
						}
					}
				})
			}
		}
	}
}

func TestReviewFormattedDueKeepsEditableCursorVisible(t *testing.T) {
	q := reviewedQuickAdd(t, "Call tomorrow")
	q.SetSize(120, 30)
	q.focusReviewField(quickAddReviewDue)
	q.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
	// Changing the seconds keeps the friendly date prefix present.
	q.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	q.Update(tea.KeyPressMsg(tea.Key{Code: '9', Text: "9"}))
	view := ansi.Strip(q.View())
	if !strings.Contains(view, "2026-09-22 00:00") || !strings.Contains(view, "20260922T000009") {
		t.Fatalf("formatted date hides editable value: %q", view)
	}
}
