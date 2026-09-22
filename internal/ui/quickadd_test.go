package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
)

func quickKey(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0]})
}

func specialQuickKey(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Mod: mod})
}

func testQuickAdd() QuickAddModel {
	q := NewQuickAdd(NewStyles(ResolveTheme("dark", true)), IconsFor("unicode"))
	q.Now = time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	q.SetSize(50, 14)
	q.OpenQuickAdd("")
	q.SetCatalog([]string{"work.client", "personal"}, []string{"planning", "client"})
	return q
}

func TestQuickAddOpensFocusedAndRefreshesContext(t *testing.T) {
	q := testQuickAdd()
	q.Input.SetValue("#wor")
	q.Input.CursorEnd()
	q.refreshSuggestions()
	if !q.Open || !q.Input.Focused() || !q.SuggestionsOpen || len(q.Suggestions) != 1 || q.Suggestions[0].Text != "#work.client" {
		t.Fatalf("quick add=%#v", q)
	}
}

func TestQuickAddArrowAndControlNavigation(t *testing.T) {
	q := testQuickAdd()
	q.Input.SetValue("#")
	q.Input.CursorEnd()
	q.refreshSuggestions()
	_, _ = q.Update(specialQuickKey(tea.KeyDown, 0))
	if q.SuggestionIndex != 1 {
		t.Fatalf("down index=%d", q.SuggestionIndex)
	}
	_, _ = q.Update(specialQuickKey('p', tea.ModCtrl))
	if q.SuggestionIndex != 0 {
		t.Fatalf("ctrl+p index=%d", q.SuggestionIndex)
	}
	_, _ = q.Update(specialQuickKey('n', tea.ModCtrl))
	if q.SuggestionIndex != 1 {
		t.Fatalf("ctrl+n index=%d", q.SuggestionIndex)
	}
}

func TestQuickAddTabAcceptsHighlightedSuggestion(t *testing.T) {
	q := testQuickAdd()
	q.Input.SetValue("#wor")
	q.Input.CursorEnd()
	q.refreshSuggestions()
	q.Update(specialQuickKey(tea.KeyTab, 0))
	if q.Input.Value() != "#work.client" || q.Input.Position() != len([]rune("#work.client")) {
		t.Fatalf("value=%q cursor=%d", q.Input.Value(), q.Input.Position())
	}
}

func TestQuickAddEstimateSuggestionAndSubmission(t *testing.T) {
	q := testQuickAdd()
	q.Input.SetValue("write docs ~")
	q.Input.CursorEnd()
	q.refreshSuggestions()
	if !q.SuggestionsOpen || len(q.Suggestions) != 6 || q.Suggestions[0].Text != "~15m" {
		t.Fatalf("suggestions=%#v", q.Suggestions)
	}
	q.Update(specialQuickKey(tea.KeyTab, 0))
	if q.Input.Value() != "write docs ~15m" {
		t.Fatalf("input=%q", q.Input.Value())
	}
	q.Input.SetValue("write docs ~1h30m")
	q.Input.CursorEnd()
	_, cmd := q.Update(specialQuickKey(tea.KeyEnter, 0))
	message, ok := cmd().(QuickAddSubmitMsg)
	if !ok || message.Task.Estimate == nil || message.Task.Estimate.Minutes != 90 {
		t.Fatalf("message=%#v", message)
	}
}

func TestQuickAddEnterEmitsParsedSubmission(t *testing.T) {
	q := testQuickAdd()
	q.Input.SetValue("write docs #work +planning")
	q.Input.CursorEnd()
	_, cmd := q.Update(specialQuickKey(tea.KeyEnter, 0))
	if cmd == nil {
		t.Fatal("enter did not return command")
	}
	message, ok := cmd().(QuickAddSubmitMsg)
	if !ok || message.Task.Description != "write docs" || message.Task.Project != "work" || len(message.Task.Tags) != 1 {
		t.Fatalf("message=%#v", message)
	}
	q.ApplyMessage(message)
	if q.Open {
		t.Fatal("successful submit should close overlay")
	}
}

func TestQuickAddParseErrorRetainsEnteredText(t *testing.T) {
	q := testQuickAdd()
	q.Input.SetValue("task #a #b")
	q.Input.CursorEnd()
	_, cmd := q.Update(specialQuickKey(tea.KeyEnter, 0))
	message, ok := cmd().(QuickAddErrorMsg)
	if !ok {
		t.Fatalf("message=%T", cmd())
	}
	q.ApplyMessage(message)
	if q.Input.Value() != "task #a #b" || q.ErrorText() == "" || !q.Open {
		t.Fatalf("q=%#v", q)
	}
}

func TestQuickAddEscapeDismissesSuggestionsThenCloses(t *testing.T) {
	q := testQuickAdd()
	q.Input.SetValue("#")
	q.Input.CursorEnd()
	q.refreshSuggestions()
	q.Update(specialQuickKey(tea.KeyEscape, 0))
	if !q.Open || q.SuggestionsOpen {
		t.Fatalf("first escape q=%#v", q)
	}
	q.Update(specialQuickKey(tea.KeyEscape, 0))
	if q.Open {
		t.Fatal("second escape should close")
	}
}

func TestQuickAddCtrlKDoesNotDeleteInput(t *testing.T) {
	q := testQuickAdd()
	q.Input.SetValue("keep this")
	q.Input.CursorEnd()
	q.Update(specialQuickKey('k', tea.ModCtrl))
	if q.Input.Value() != "keep this" {
		t.Fatalf("input changed to %q", q.Input.Value())
	}
}

func TestQuickAddModalRendersSuggestionsAndGuidanceWithinBounds(t *testing.T) {
	q := testQuickAdd()
	q.Input.SetValue("#")
	q.Input.CursorEnd()
	q.refreshSuggestions()
	got := q.View()
	lines := strings.Split(got, "\n")
	if len(lines) > q.Height || len(lines) < 2 {
		t.Fatalf("lines=%d view=%q", len(lines), got)
	}
	for _, want := range []string{"Quick capture", "Task", "Projects", "#work.client", "Enter add task", "Esc cancel", "Tab use"} {
		if !strings.Contains(got, want) {
			t.Errorf("modal missing %q: %q", want, got)
		}
	}
	for _, unwanted := range []string{"Optional details", "Example:", "!priority", "@due"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("suggestions should replace static guidance; found %q: %q", unwanted, got)
		}
	}
	if strings.Index(got, "#work.client") <= strings.Index(got, "> ") {
		t.Fatalf("suggestion should render below input: %q", got)
	}
}

func TestQuickAddIdleStateShowsCompactSyntaxGuide(t *testing.T) {
	q := testQuickAdd()
	got := q.View()
	for _, want := range []string{"Optional details", "#project", "!priority", "@due date", ">scheduled", "+tag", "~estimate"} {
		if !strings.Contains(got, want) {
			t.Errorf("guide missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "Example:") {
		t.Fatalf("idle guide should stay compact: %q", got)
	}
}

func TestQuickAddSuggestionsAreCappedForShortTerminal(t *testing.T) {
	q := testQuickAdd()
	q.SetSize(20, 2)
	q.Input.SetValue("@")
	q.Input.CursorEnd()
	q.refreshSuggestions()
	if lines := strings.Count(q.View(), "\n") + 1; lines > 2 {
		t.Fatalf("lines=%d", lines)
	}
}

func TestQuickAddSetCatalogCopiesValues(t *testing.T) {
	q := testQuickAdd()
	projects := []string{"work"}
	tags := []string{"tag"}
	q.SetCatalog(projects, tags)
	projects[0] = "changed"
	tags[0] = "changed"
	if q.Projects[0] != "work" || q.Tags[0] != "tag" {
		t.Fatalf("catalog was not copied: %#v", q)
	}
}

func TestQuickAddApplyMessageOnlyHandlesOwnMessages(t *testing.T) {
	q := testQuickAdd()
	if message, handled := q.ApplyMessage(domain.NewTask{Description: "not a tea message"}); handled || message != nil {
		t.Fatalf("unexpected handling: %#v %v", message, handled)
	}
}

func TestQuickAddCloseClearsSuggestionsAndBlurs(t *testing.T) {
	q := testQuickAdd()
	q.Input.SetValue("#")
	q.Input.CursorEnd()
	q.refreshSuggestions()
	q.Close()
	if q.Open || q.SuggestionsOpen || len(q.Suggestions) != 0 || q.Input.Focused() {
		t.Fatalf("close state=%#v", q)
	}
}
