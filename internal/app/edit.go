package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/ui"
)

// EditShortcutField maps the fixed v1 task-list shortcuts to editor fields.
func EditShortcutField(key string) (ui.EditField, bool) {
	switch key {
	case "e":
		return ui.FieldDescription, true
	case "p":
		return ui.FieldProject, true
	case "!":
		return ui.FieldPriority, true
	case "D":
		return ui.FieldDue, true
	case "S":
		return ui.FieldScheduled, true
	case "t":
		return ui.FieldTags, true
	default:
		return ui.FieldDescription, false
	}
}

// OpenEditShortcut applies one fixed shortcut without invoking Taskwarrior.
func (m *Model) OpenEditShortcut(key string) tea.Cmd {
	field, ok := EditShortcutField(key)
	if !ok {
		return nil
	}
	return m.OpenEditor(field)
}

// SubmitEdit validates the app-level target and starts exactly one modify
// operation for a non-empty supported diff.
func (m *Model) SubmitEdit(message ui.EditSubmitMsg) tea.Cmd {
	return m.handleEditSubmit(message)
}

// EditErrorText returns a concise error suitable for the modal/footer.
func EditErrorText(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if len(text) > 160 {
		return text[:157] + "..."
	}
	return text
}
