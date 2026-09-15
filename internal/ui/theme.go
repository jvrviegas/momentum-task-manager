package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum/internal/config"
)

// Theme is a named palette. Values are ANSI-compatible color strings so tests
// can inspect the selected variant without depending on terminal capabilities.
type Theme struct {
	Name      string
	Surface   string
	Panel     string
	Text      string
	Muted     string
	Selection string
	Accent    string
	Cyan      string
	Overdue   string
	High      string
	Border    string
}

func darkTheme() Theme {
	return Theme{
		Name: "dark", Surface: "#161a2b", Panel: "#1f2438", Text: "#c0caf5", Muted: "#7982a9",
		Selection: "#283457", Accent: "#7aa2f7", Cyan: "#7dcfff", Overdue: "#f7768e", High: "#ff9e64", Border: "#3b4261",
	}
}

func lightTheme() Theme {
	return Theme{
		Name: "light", Surface: "#f6f7fb", Panel: "#e8ebf3", Text: "#343b58", Muted: "#69708b",
		Selection: "#d7e3ff", Accent: "#34548a", Cyan: "#0f7b8f", Overdue: "#c53b53", High: "#b15c00", Border: "#a9b1c6",
	}
}

// ResolveTheme chooses a deterministic palette. auto uses the supplied
// terminal observation; callers can obtain that observation at startup.
func ResolveTheme(mode string, darkBackground bool) Theme {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case config.ThemeLight:
		return lightTheme()
	case config.ThemeDark:
		return darkTheme()
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

// Styles contains shared Lip Gloss styles used by all components.
type Styles struct {
	Title     lipgloss.Style
	Muted     lipgloss.Style
	Project   lipgloss.Style
	Overdue   lipgloss.Style
	Priority  lipgloss.Style
	Selection lipgloss.Style
	Panel     lipgloss.Style
	Border    lipgloss.Style
}

// NewStyles creates the common style vocabulary for a palette.
func NewStyles(theme Theme) Styles {
	return Styles{
		Title:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Text)),
		Muted:     lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted)),
		Project:   lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Cyan)),
		Overdue:   lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Overdue)),
		Priority:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.High)),
		Selection: lipgloss.NewStyle().Background(lipgloss.Color(theme.Selection)).Foreground(lipgloss.Color(theme.Text)),
		Panel:     lipgloss.NewStyle().Background(lipgloss.Color(theme.Panel)),
		Border:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(theme.Border)),
	}
}
