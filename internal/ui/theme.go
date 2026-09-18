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
	Medium    string
	Low       string
	Border    string
}

func darkTheme() Theme {
	return Theme{
		Name: "dark", Surface: "#161a2b", Panel: "#1f2438", Text: "#c0caf5", Muted: "#7982a9",
		Selection: "#283457", Accent: "#7aa2f7", Cyan: "#7dcfff", Overdue: "#f7768e", High: "#ff9e64",
		Medium: "#e0af68", Low: "#7982a9", Border: "#3b4261",
	}
}

func lightTheme() Theme {
	return Theme{
		Name: "light", Surface: "#f6f7fb", Panel: "#e8ebf3", Text: "#343b58", Muted: "#69708b",
		Selection: "#d7e3ff", Accent: "#34548a", Cyan: "#0f7b8f", Overdue: "#c53b53", High: "#b15c00",
		Medium: "#8a6500", Low: "#69708b", Border: "#a9b1c6",
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

// Styles contains shared Lip Gloss styles used by all components. The older
// roles remain alongside the readability vocabulary so small embedders can
// migrate without a flag or palette fork.
type Styles struct {
	Title     lipgloss.Style
	Muted     lipgloss.Style
	Project   lipgloss.Style
	Overdue   lipgloss.Style
	Priority  lipgloss.Style
	Selection lipgloss.Style
	Panel     lipgloss.Style
	Border    lipgloss.Style

	PageTitle    lipgloss.Style
	SectionTitle lipgloss.Style
	Metadata     lipgloss.Style
	FocusGutter  lipgloss.Style
	KeyHint      lipgloss.Style
	Status       lipgloss.Style
	Error        lipgloss.Style
	Active       lipgloss.Style
	FieldLabel   lipgloss.Style
	FieldValue   lipgloss.Style
	ModalTitle   lipgloss.Style
	ModalBody    lipgloss.Style
	ModalAction  lipgloss.Style
	PriorityHigh lipgloss.Style
	PriorityMed  lipgloss.Style
	PriorityLow  lipgloss.Style
}

// NewStyles creates the common style vocabulary for a palette. Components
// should select a role here rather than introduce a one-off color literal.
func NewStyles(theme Theme) Styles {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Text))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))
	section := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Accent))
	metadata := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))
	gutter := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Accent))
	keyHint := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Text))
	status := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Text))
	errorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Overdue))
	active := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Cyan))
	fieldLabel := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted))
	fieldValue := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Text))
	modalTitle := title
	modalBody := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Text))
	modalAction := keyHint

	panel := lipgloss.NewStyle().Background(lipgloss.Color(theme.Panel))
	border := panel.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(theme.Border))

	return Styles{
		Title:     title,
		Muted:     muted,
		Project:   lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Cyan)),
		Overdue:   errorStyle,
		Priority:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.High)),
		Selection: lipgloss.NewStyle().Background(lipgloss.Color(theme.Selection)).Foreground(lipgloss.Color(theme.Text)),
		Panel:     panel,
		Border:    border,

		PageTitle:    title,
		SectionTitle: section,
		Metadata:     metadata,
		FocusGutter:  gutter,
		KeyHint:      keyHint,
		Status:       status,
		Error:        errorStyle,
		Active:       active,
		FieldLabel:   fieldLabel,
		FieldValue:   fieldValue,
		ModalTitle:   modalTitle,
		ModalBody:    modalBody,
		ModalAction:  modalAction,
		PriorityHigh: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.High)),
		PriorityMed:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Medium)),
		PriorityLow:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Low)),
	}
}

// RenderModal is the shared bordered shell for bounded overlays. lines are
// treated as already wrapped; it preserves the title/body/action hierarchy
// while guaranteeing a positive inner width.
func RenderModal(lines []string, width, height int, styles Styles) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	contentWidth := ModalContentWidth(width, 0)
	contentHeight := ModalContentHeight(height, 0)
	if len(lines) == 0 {
		lines = []string{""}
	}
	return renderBoundedPanel(lines, contentWidth, contentHeight, styles)
}

func renderBoundedPanel(lines []string, contentWidth, contentHeight int, styles Styles) string {
	if contentWidth <= 0 || contentHeight <= 0 {
		return ""
	}
	if contentWidth < 2 || contentHeight < 2 {
		if len(lines) == 0 {
			return ""
		}
		return Truncate(lines[0], contentWidth)
	}
	if len(lines) > contentHeight {
		if contentHeight == 1 {
			lines = lines[:1]
		} else {
			lines = append(append([]string(nil), lines[:1]...), lines[len(lines)-(contentHeight-1):]...)
		}
	}
	for index := range lines {
		lines[index] = PadRight(Truncate(lines[index], contentWidth), contentWidth)
	}
	body := styles.ModalBody.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	// In Lip Gloss v2 Width includes the border box. Adding the two border
	// cells prevents the shell from wrapping already-wrapped body lines.
	return styles.Border.Width(contentWidth + 2).Render(body)
}
