package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jvrviegas/momentum-task-manager/internal/config"
)

func appearanceKey(code rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: code}) }

func TestAppearanceCyclesThemesInBothDirections(t *testing.T) {
	model := NewAppearanceSettings(NewStyles(darkTheme()), "")
	if model.Saved != config.ThemeAuto {
		t.Fatalf("empty theme should start at auto, got %q", model.Saved)
	}
	model.Update(appearanceKey(tea.KeyLeft))
	if model.Draft != config.ThemeTerminal || !model.Dirty() {
		t.Fatalf("left from auto should wrap to terminal, got %q", model.Draft)
	}
	model.Update(appearanceKey(tea.KeyRight))
	if model.Draft != config.ThemeAuto || model.Dirty() {
		t.Fatalf("right should wrap back to auto, got %q", model.Draft)
	}
}

func TestAppearanceSavesOnlyDirtyDraftsAndIgnoresKeysWhileSaving(t *testing.T) {
	model := NewAppearanceSettings(NewStyles(darkTheme()), config.ThemeDark)
	if cmd := model.Update(appearanceKey(tea.KeyEnter)); cmd != nil {
		t.Fatal("clean enter should not save")
	}
	model.Update(appearanceKey(tea.KeyRight))
	cmd := model.Update(appearanceKey(tea.KeyEnter))
	if cmd == nil || !model.Saving {
		t.Fatal("dirty enter did not start a save")
	}
	if msg, ok := cmd().(AppearanceSaveMsg); !ok || msg.Theme != config.ThemeLight {
		t.Fatalf("intent=%#v", cmd())
	}
	model.Update(appearanceKey(tea.KeyRight))
	if model.Draft != config.ThemeLight {
		t.Fatal("draft changed during save")
	}
	model.ApplySaved(errors.New("conflict"))
	if model.Saving || model.Err == nil || !model.Dirty() {
		t.Fatalf("failed save state=%#v", model)
	}
	model.Update(appearanceKey(tea.KeyEnter))
	model.ApplySaved(nil)
	if model.Saved != config.ThemeLight || model.Dirty() || model.Err != nil {
		t.Fatalf("saved state=%#v", model)
	}
}

func TestAppearanceRendersWithinWidthAtEveryLayout(t *testing.T) {
	for _, width := range []int{120, 79, 49} {
		model := NewAppearanceSettings(NewStyles(darkTheme()), config.ThemeAuto)
		model.Update(appearanceKey(tea.KeyLeft))
		model.Err = errors.New("project config changed since snapshot")
		view := model.ViewAt(width, 18)
		for index, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width=%d line %d is %d cells: %q", width, index, got, ansi.Strip(line))
			}
		}
		plain := ansi.Strip(view)
		for _, want := range []string{"Settings / Appearance", "‹ terminal ›", "16 ANSI colors", "unsaved", "discard"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("width=%d missing %q in:\n%s", width, want, plain)
			}
		}
		if (width >= NarrowBreakpoint) != strings.Contains(plain, "was auto") {
			t.Fatalf("width=%d was-hint mismatch:\n%s", width, plain)
		}
	}
}
