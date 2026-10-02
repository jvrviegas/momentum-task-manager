package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/config"
)

// Theme is a named palette. Values are ANSI-compatible color strings so tests
// can inspect the selected variant without depending on terminal capabilities.
//
// Surface is documentation only: Momentum never paints it, so a transparent
// terminal background keeps working. Panel and Selection are the only fills.
// MutedFill is Muted text drawn on Panel or Selection, and Dim redraws the
// view behind a modal in one color.
type Theme struct {
	Name      string
	Surface   string
	Panel     string
	Text      string
	Muted     string
	MutedFill string
	Selection string
	Accent    string
	Cyan      string
	Overdue   string
	High      string
	Medium    string
	Low       string
	Green     string
	Purple    string
	Teal      string
	Border    string
	Dim       string
}

func darkTheme() Theme {
	return Theme{
		Name: "dark", Surface: "#161a2b", Panel: "#1f2438", Text: "#c0caf5", Muted: "#7982a9", MutedFill: "#959ec8",
		Selection: "#283457", Accent: "#7aa2f7", Cyan: "#7dcfff", Overdue: "#f7768e", High: "#ff9e64",
		Medium: "#e0af68", Low: "#7982a9", Green: "#9ece6a", Purple: "#bb9af7", Teal: "#73daca",
		Border: "#3b4261", Dim: "#4a5170",
	}
}

func lightTheme() Theme {
	return Theme{
		Name: "light", Surface: "#f6f7fb", Panel: "#e8ebf3", Text: "#343b58", Muted: "#5a6180", MutedFill: "#5a6180",
		Selection: "#d7e3ff", Accent: "#34548a", Cyan: "#0b6b7d", Overdue: "#b03049", High: "#9a4d00",
		Medium: "#7a5800", Low: "#5a6180", Green: "#4f6b2f", Purple: "#7343b5", Teal: "#1d6b5f",
		Border: "#a9b1c6", Dim: "#b4bacb",
	}
}

// terminalTheme uses the terminal's 16 ANSI colors so its color scheme
// decides every hue. Text is unset to keep the default foreground; the
// neutral fills, Muted-on-fill and Surface follow the background observation.
func terminalTheme(darkBackground bool) Theme {
	theme := Theme{
		Name: "terminal", Text: "", Muted: "8", Accent: "4", Cyan: "6", Overdue: "1", High: "9",
		Medium: "3", Low: "8", Green: "2", Purple: "5", Teal: "14", Border: "8",
	}
	if darkBackground {
		theme.Surface, theme.Panel, theme.Selection, theme.MutedFill, theme.Dim = "0", "0", "8", "7", "8"
	} else {
		theme.Surface, theme.Panel, theme.Selection, theme.MutedFill, theme.Dim = "15", "15", "7", "0", "7"
	}
	return theme
}

// ResolveTheme chooses a deterministic palette. auto and terminal use the
// supplied terminal observation; callers can obtain it at startup.
func ResolveTheme(mode string, darkBackground bool) Theme {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case config.ThemeLight:
		return lightTheme()
	case config.ThemeDark:
		return darkTheme()
	case config.ThemeTerminal:
		return terminalTheme(darkBackground)
	default:
		if darkBackground {
			return darkTheme()
		}
		return lightTheme()
	}
}

// ThemeFromConfig resolves a configured theme with an explicit background
// observation, which keeps rendering tests free of terminal I/O.
func ThemeFromConfig(settings config.Config, darkBackground bool) Theme {
	return ResolveTheme(settings.Theme, darkBackground)
}

// Styles carries the palette for the span painter. Components select a Tone
// or Fill rather than introduce a one-off color literal.
type Styles struct {
	Theme Theme
}

// NewStyles creates the painter for a palette.
func NewStyles(theme Theme) Styles {
	return Styles{Theme: theme}
}

func (s Styles) tone(t Tone, onFill bool) color.Color {
	theme := s.Theme
	value := theme.Text
	switch t {
	case ToneMuted:
		value = theme.Muted
		if onFill {
			value = theme.MutedFill
		}
	case ToneLow:
		value = theme.Low
		if onFill {
			value = theme.MutedFill
		}
	case ToneAccent:
		value = theme.Accent
	case ToneCyan:
		value = theme.Cyan
	case ToneRed:
		value = theme.Overdue
	case ToneHigh:
		value = theme.High
	case ToneMedium:
		value = theme.Medium
	case ToneGreen:
		value = theme.Green
	case TonePurple:
		value = theme.Purple
	case ToneTeal:
		value = theme.Teal
	case ToneBorder:
		value = theme.Border
	case ToneSurface:
		value = theme.Surface
	case ToneDim:
		value = theme.Dim
	}
	return lipgloss.Color(value)
}

func (s Styles) fill(f Fill) color.Color {
	switch f {
	case FillPanel:
		return lipgloss.Color(s.Theme.Panel)
	case FillSelection:
		return lipgloss.Color(s.Theme.Selection)
	case FillAccent:
		return lipgloss.Color(s.Theme.Accent)
	case FillRed:
		return lipgloss.Color(s.Theme.Overdue)
	default:
		return nil
	}
}
