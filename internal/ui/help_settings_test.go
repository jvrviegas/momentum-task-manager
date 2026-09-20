package ui

import (
	"strings"
	"testing"
)

func TestHelpIncludesSettingsShortcut(t *testing.T) {
	if !strings.Contains(HelpText(), "1 / 2 / 3 / 4 — Inbox / Today / Completed / Settings") {
		t.Fatalf("help=%q", HelpText())
	}
}
