package ui

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/internal/config"
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

func TestTerminalThemeUsesOnlyANSIColors(t *testing.T) {
	for _, dark := range []bool{true, false} {
		theme := ResolveTheme(config.ThemeTerminal, dark)
		if theme.Name != "terminal" || theme.Text != "" {
			t.Fatalf("dark=%v theme=%#v", dark, theme)
		}
		values := reflect.ValueOf(theme)
		for index := 0; index < values.NumField(); index++ {
			name := values.Type().Field(index).Name
			value := values.Field(index).String()
			if name == "Name" || name == "Text" {
				continue
			}
			if number, err := strconv.Atoi(value); err != nil || number < 0 || number > 15 {
				t.Errorf("dark=%v %s=%q is not one of the 16 ANSI colors", dark, name, value)
			}
		}
	}
	if dark, light := ResolveTheme(config.ThemeTerminal, true), ResolveTheme(config.ThemeTerminal, false); dark.Selection == light.Selection || dark.MutedFill == light.MutedFill {
		t.Fatal("terminal fills did not follow the background observation")
	}
}

func TestTerminalThemeKeepsDefaultForegroundAndReversesCursor(t *testing.T) {
	styles := NewStyles(ResolveTheme(config.ThemeTerminal, true))
	if got := styles.Line(4, FillNone, txt("text")); got != "text" {
		t.Fatalf("text should use the default foreground, got %q", got)
	}
	if got := styles.Line(1, FillNone, Span{Text: " ", Cursor: true}); !strings.Contains(got, "\x1b[7m") {
		t.Fatalf("cursor should use reverse video, got %q", got)
	}
}

func TestStylesUsePaletteColors(t *testing.T) {
	theme := ResolveTheme(config.ThemeDark, true)
	styles := NewStyles(theme)
	if styles.Theme != theme || styles.tone(ToneMuted, false) == nil || styles.fill(FillSelection) == nil {
		t.Fatal("shared styles did not receive palette colors")
	}
}

func TestPaletteCarriesFillAndBackdropTokens(t *testing.T) {
	dark := ResolveTheme(config.ThemeDark, true)
	light := ResolveTheme(config.ThemeLight, true)
	if dark.MutedFill != "#959ec8" || dark.Dim != "#4a5170" || light.Dim != "#b4bacb" {
		t.Fatalf("missing new tokens: dark=%#v light=%#v", dark, light)
	}
	// Light tokens are darkened to clear 4.5:1 on Panel and Selection.
	if light.Muted != "#5a6180" || light.Cyan != "#0b6b7d" || light.Overdue != "#b03049" || light.High != "#9a4d00" || light.Medium != "#7a5800" ||
		light.Green != "#4f6b2f" || light.Purple != "#7343b5" || light.Teal != "#1d6b5f" {
		t.Fatalf("light palette=%#v", light)
	}
}

func TestMutedTextSwitchesToMutedFillOnFills(t *testing.T) {
	styles := NewStyles(ResolveTheme(config.ThemeDark, true))
	plain := styles.Line(4, FillNone, muted("meta"))
	filled := styles.Line(4, FillSelection, muted("meta"))
	if plain == filled || !strings.Contains(filled, "meta") {
		t.Fatalf("plain=%q filled=%q", plain, filled)
	}
}

func TestIconModesHaveDistinctSafeSets(t *testing.T) {
	unicodeSet := IconsFor(config.IconsUnicode)
	nerdSet := IconsFor(config.IconsNerd)
	asciiSet := IconsFor(config.IconsASCII)
	if unicodeSet.Pending == asciiSet.Pending || nerdSet.Pending == unicodeSet.Pending || asciiSet.Search != "/" {
		t.Fatalf("icon modes not distinct: %#v %#v %#v", unicodeSet, nerdSet, asciiSet)
	}
	values := reflect.ValueOf(asciiSet)
	for index := 0; index < values.NumField(); index++ {
		value, ok := values.Field(index).Interface().(string)
		if !ok {
			continue
		}
		for _, r := range value {
			if r > 0x7e || r < 0x20 {
				t.Fatalf("ascii %s=%q is not printable ASCII", values.Type().Field(index).Name, value)
			}
		}
	}
	if asciiSet.Inbox != "" || asciiSet.Active != "[>]" || asciiSet.SelectionCont != " " {
		t.Fatalf("ascii navigation/state glyphs=%#v", asciiSet)
	}
	if nerdSet.Rule != unicodeSet.Rule || nerdSet.Selection != unicodeSet.Selection || nerdSet.CornerTL != unicodeSet.CornerTL {
		t.Fatal("nerd mode must keep unicode chrome")
	}
}

func TestUnknownIconModeFallsBackToUnicode(t *testing.T) {
	if IconsFor("unknown") != IconsFor(config.IconsUnicode) {
		t.Fatal("unknown mode should use unicode fallback")
	}
}

func TestNewStylesCanRender(t *testing.T) {
	styles := NewStyles(ResolveTheme(config.ThemeDark, true))
	if got := styles.Line(12, FillNone, txt("Momentum").bold()); got == "" || !strings.Contains(got, "Momentum") {
		t.Fatalf("rendered title=%q", got)
	}
}
