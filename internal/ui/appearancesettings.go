package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/config"
)

// ThemeChoices is the order ←→ cycles themes in Settings → Appearance.
var ThemeChoices = []string{config.ThemeAuto, config.ThemeDark, config.ThemeLight, config.ThemeTerminal}

var themeDescriptions = map[string]string{
	config.ThemeAuto:     "Dark or light palette from the terminal background",
	config.ThemeDark:     "Tokyo Night-inspired dark palette",
	config.ThemeLight:    "Tokyo Night-inspired light palette",
	config.ThemeTerminal: "Your terminal's 16 ANSI colors",
}

// AppearanceSaveMsg is the explicit theme-save intent sent to the app.
type AppearanceSaveMsg struct{ Theme string }

// AppearanceCloseMsg asks the app to leave Settings from Appearance.
type AppearanceCloseMsg struct{}

// AppearanceSettingsModel edits the theme draft. The app previews Draft live
// and persists it; the model performs no I/O.
type AppearanceSettingsModel struct {
	Saved  string
	Draft  string
	Saving bool
	Err    error

	Width  int
	Height int
	Styles Styles
}

// NewAppearanceSettings creates the editor for a configured theme.
func NewAppearanceSettings(styles Styles, theme string) AppearanceSettingsModel {
	a := AppearanceSettingsModel{Styles: styles}
	a.Reset(theme)
	return a
}

// Reset makes theme both the saved value and the draft.
func (a *AppearanceSettingsModel) Reset(theme string) {
	theme = strings.ToLower(strings.TrimSpace(theme))
	if themeIndex(theme) < 0 {
		theme = config.ThemeAuto
	}
	a.Saved, a.Draft, a.Saving, a.Err = theme, theme, false, nil
}

// Dirty reports an unsaved theme draft.
func (a AppearanceSettingsModel) Dirty() bool { return a.Draft != a.Saved }

// ApplySaved records a save result; a failure keeps the draft.
func (a *AppearanceSettingsModel) ApplySaved(err error) {
	a.Saving = false
	if err != nil {
		a.Err = err
		return
	}
	a.Reset(a.Draft)
}

// Update cycles, saves, or discards the draft.
func (a *AppearanceSettingsModel) Update(key tea.KeyPressMsg) tea.Cmd {
	if a.Saving {
		return nil
	}
	switch key.String() {
	case "left", "h":
		a.cycle(-1)
	case "right", "l":
		a.cycle(1)
	case "enter", "s":
		if a.Dirty() {
			a.Saving, a.Err = true, nil
			return settingsMessageCommand(AppearanceSaveMsg{Theme: a.Draft})
		}
	case "esc", "escape":
		if a.Dirty() {
			a.Draft, a.Err = a.Saved, nil
			return nil
		}
		return settingsMessageCommand(AppearanceCloseMsg{})
	}
	return nil
}

func (a *AppearanceSettingsModel) cycle(delta int) {
	index := (themeIndex(a.Draft) + delta + len(ThemeChoices)) % len(ThemeChoices)
	a.Draft, a.Err = ThemeChoices[index], nil
}

func themeIndex(theme string) int {
	for index, choice := range ThemeChoices {
		if choice == theme {
			return index
		}
	}
	return -1
}

// ViewAt renders the section at the supplied body size.
func (a AppearanceSettingsModel) ViewAt(width, height int) string {
	a.Width, a.Height = width, height
	if width <= 0 || height <= 0 {
		return ""
	}
	icons := Icons{}.orUnicode()
	frameWidth := min(SettingsWidth, max(1, width))
	content := FrameContentWidth(frameWidth)
	labelWidth := 12
	if width < NarrowBreakpoint {
		labelWidth = 10
	}
	valueColumn := 2 + labelWidth + 1

	marker := txt(" ")
	if a.Dirty() {
		marker = sp(icons.Changed, ToneAccent)
	}
	if a.Err != nil {
		marker = sp(icons.Error, ToneRed).bold()
	}
	value := []Span{marker, txt(" "), txt(fmt.Sprintf("%-*s", labelWidth, "Theme")).bold(), txt(" "), txt("‹ " + a.Draft + " ›").bold()}
	if a.Dirty() && width >= NarrowBreakpoint {
		value = append(value, muted("   was "+a.Saved))
	}
	choices := []Span{gap(valueColumn)}
	for index, choice := range ThemeChoices {
		if index > 0 {
			choices = append(choices, muted(" "+icons.Dot+" "))
		}
		if choice == a.Draft {
			choices = append(choices, sp(choice, ToneAccent).bold())
		} else {
			choices = append(choices, muted(choice))
		}
	}
	rows := []FrameRow{
		{},
		{Spans: append(value, Span{Text: " ", Grow: true}), Mark: icons.Selection, Bg: FillSelection},
		row(choices...),
		row(gap(valueColumn), muted(themeDescriptions[a.Draft])),
	}
	if a.Err != nil {
		for _, line := range WrapText(a.Err.Error(), max(1, content-valueColumn)) {
			rows = append(rows, row(gap(valueColumn), sp(line, ToneRed)))
		}
	}
	rows = append(rows, FrameRow{})

	keys := []Hint{{"←→", "change"}, {"esc", "close"}, {"tab", "projects"}}
	status := muted("saved")
	switch {
	case a.Saving:
		status = muted("saving…")
	case a.Dirty():
		keys = []Hint{{"←→", "change"}, {"enter", "save"}, {"esc", "discard"}}
		status = sp("unsaved", ToneAccent)
	}
	frame := Frame{
		Title: "Settings / Appearance", Context: []Span{status},
		Rows: rows, Width: frameWidth, MaxRows: max(1, height-2), Keys: keys,
	}
	return strings.Join(a.Styles.RenderFrame(frame, icons), "\n")
}
