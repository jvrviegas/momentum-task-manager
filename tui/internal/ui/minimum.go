package ui

import (
	"fmt"
	"strings"
)

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
