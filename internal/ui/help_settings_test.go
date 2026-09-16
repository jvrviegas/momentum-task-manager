package ui

import (
	"strings"
	"testing"
)

func TestHelpIncludesSettingsShortcut(t *testing.T) {
	if !strings.Contains(HelpText(), "1 / 2 / 3 — Inbox / Today / Settings") {
		t.Fatalf("help=%q", HelpText())
	}
}
