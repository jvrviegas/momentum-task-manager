package ui

import (
	"strings"
	"testing"

	"github.com/jvrviegas/momentum/internal/config"
)

func TestThemeModesResolveDeterministically(t *testing.T) {
	if ResolveTheme(config.ThemeDark, false).Name != "dark" {
		t.Fatal("dark override did not win")
	}
	if ResolveTheme(config.ThemeLight, true).Name != "light" {
		t.Fatal("light override did not win")
	}
	if ResolveTheme(config.ThemeAuto, true).Name != "dark" || ResolveTheme(config.ThemeAuto, false).Name != "light" {
		t.Fatal("auto theme did not follow explicit background observation")
	}
}

func TestStylesUsePaletteColors(t *testing.T) {
	theme := ResolveTheme(config.ThemeDark, true)
	styles := NewStyles(theme)
	if styles.Title.GetForeground() == nil || styles.Project.GetForeground() == nil || styles.Selection.GetBackground() == nil {
		t.Fatal("shared styles did not receive palette colors")
	}
}

func TestIconModesHaveDistinctSafeSets(t *testing.T) {
	unicodeSet := IconsFor(config.IconsUnicode)
	nerdSet := IconsFor(config.IconsNerd)
	asciiSet := IconsFor(config.IconsASCII)
	if unicodeSet.Pending == asciiSet.Pending || nerdSet.Pending == unicodeSet.Pending || asciiSet.Search != "/" {
		t.Fatalf("icon modes not distinct: %#v %#v %#v", unicodeSet, nerdSet, asciiSet)
	}
	if strings.Contains(asciiSet.Pending, "□") {
		t.Fatal("ascii set contains unicode glyph")
	}
}

func TestUnknownIconModeFallsBackToUnicode(t *testing.T) {
	if IconsFor("unknown") != IconsFor(config.IconsUnicode) {
		t.Fatal("unknown mode should use unicode fallback")
	}
}

func TestNewStylesCanRender(t *testing.T) {
	styles := NewStyles(ResolveTheme(config.ThemeDark, true))
	if got := styles.Title.Render("Momentum"); got == "" || !strings.Contains(got, "Momentum") {
		t.Fatalf("rendered title=%q", got)
	}
}
