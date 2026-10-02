package ui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func modalKey(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0]})
}

func TestDetailsShowsSupportedAndUnknownFields(t *testing.T) {
	details := NewDetails(NewStyles(ResolveTheme("dark", true)))
	details.SetSize(70, 20)
	details.OpenTask(domain.Task{
		UUID: "uuid", ID: 7, Description: "Task", Status: "pending", Project: "work", Priority: "H",
		Tags: []string{"one"}, Dependencies: []string{"other"}, Recurrence: "weekly", Urgency: 4.25,
		RawFields: map[string]json.RawMessage{"description": json.RawMessage(`"Task"`), "custom": json.RawMessage(`"value"`)},
	})
	view := details.View()
	for i := 0; i < 40; i++ {
		details.Update(modalKey("down"))
	}
	view += details.View()
	for _, want := range []string{"Task details", "OVERVIEW", "SCHEDULE", "RECORD", "Task", "#work", "weekly", "uuid", "custom", "value"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q: %q", want, view)
		}
	}
}

func TestDetailsEscEnterCloseAndEdit(t *testing.T) {
	details := NewDetails(Styles{})
	details.OpenTask(domain.Task{Description: "Task"})
	if action := details.Update(modalKey("e")); action != DetailsEdit || details.Open {
		t.Fatalf("edit action=%s open=%v", action, details.Open)
	}
	details.OpenTask(domain.Task{Description: "Task"})
	if action := details.Update(modalKey("esc")); action != DetailsClose || details.Open {
		t.Fatalf("close action=%s open=%v", action, details.Open)
	}
	details.OpenTask(domain.Task{Description: "Task"})
	if action := details.Update(modalKey("enter")); action != DetailsClose || details.Open {
		t.Fatalf("enter action=%s open=%v", action, details.Open)
	}
}

func TestDetailsIgnoresKeysWhenClosed(t *testing.T) {
	details := NewDetails(Styles{})
	if action := details.Update(modalKey("e")); action != DetailsNone {
		t.Fatalf("action=%s", action)
	}
}

func TestConfirmRequiresExplicitYesOrNo(t *testing.T) {
	confirm := NewConfirm(NewStyles(ResolveTheme("dark", true)))
	confirm.SetSize(40, 8)
	confirm.OpenFor("Delete task", "Delete this task permanently?")
	if action := confirm.Update(modalKey("x")); action != ConfirmNone || !confirm.Open {
		t.Fatal("unknown key should not confirm")
	}
	if action := confirm.Update(modalKey("n")); action != ConfirmNo || confirm.Open {
		t.Fatal("n should cancel confirmation")
	}
	confirm.OpenFor("Delete task", "Delete?")
	if action := confirm.Update(modalKey("y")); action != ConfirmYes || confirm.Open {
		t.Fatal("y should confirm")
	}
}

func TestConfirmEscapeCancels(t *testing.T) {
	confirm := NewConfirm(Styles{})
	confirm.OpenFor("Delete", "?")
	if action := confirm.Update(modalKey("esc")); action != ConfirmCancel || confirm.Open {
		t.Fatalf("action=%s open=%v", action, confirm.Open)
	}
}

func TestHelpDerivesTextFromBindings(t *testing.T) {
	text := HelpText()
	for _, want := range []string{"ctrl+k — capture", "ctrl+r — sync now", "space — complete", "click — select"} {
		if !strings.Contains(text, want) {
			t.Errorf("help missing %q", want)
		}
	}
	if len(KeyBindings()) < 15 {
		t.Fatalf("bindings=%d", len(KeyBindings()))
	}
}

func TestHelpOpensRendersAndCloses(t *testing.T) {
	help := NewHelp(NewStyles(ResolveTheme("dark", true)))
	help.SetSize(70, 20)
	help.OpenHelp()
	if view := help.View(); !strings.Contains(view, "Keys") || !strings.Contains(view, "NAVIGATE") {
		t.Fatalf("view=%q", view)
	}
	if action := help.Update(modalKey("?")); action != HelpClose || help.Open {
		t.Fatalf("action=%s open=%v", action, help.Open)
	}
}

func TestHelpTruncatesToModalHeight(t *testing.T) {
	view := RenderHelp(30, 4, Styles{})
	if lines := strings.Count(view, "\n") + 1; lines > 4 {
		t.Fatalf("lines=%d", lines)
	}
}

func TestQuitChoicesMatchShutdownCopy(t *testing.T) {
	quit := NewQuit(NewStyles(ResolveTheme("dark", true)))
	quit.SetSize(50, 10)
	quit.OpenQuit()
	view := quit.View()
	for _, want := range []string{"Quit Momentum?", "Unsynced changes", "Sync and quit", "Quit without syncing", "Cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("quit view missing %q", want)
		}
	}
	if choice := quit.Update(modalKey("s")); choice != QuitSync || quit.Open {
		t.Fatalf("sync choice=%s open=%v", choice, quit.Open)
	}
	quit.OpenQuit()
	if choice := quit.Update(modalKey("q")); choice != QuitLocal {
		t.Fatalf("local choice=%s", choice)
	}
	quit.OpenQuit()
	if choice := quit.Update(modalKey("esc")); choice != QuitCancel {
		t.Fatalf("cancel choice=%s", choice)
	}
}

func TestMinimumSizeWarningIsSafe(t *testing.T) {
	if got := MinimumSizeMessage(20, 5, Styles{}); got == "" || !strings.Contains(got, "Terminal") {
		t.Fatalf("warning=%q", got)
	}
	if got := MinimumSizeMessage(0, 0, Styles{}); got != "" {
		t.Fatalf("warning=%q", got)
	}
}
