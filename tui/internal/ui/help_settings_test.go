package ui

import (
	"strings"
	"testing"
)

func TestHelpIncludesSettingsShortcut(t *testing.T) {
	if !strings.Contains(HelpText(), "1-4 — jump to view") || !strings.Contains(HelpText(), "4 — settings") {
		t.Fatalf("help=%q", HelpText())
	}
}
