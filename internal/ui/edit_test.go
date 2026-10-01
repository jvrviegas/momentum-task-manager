package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
)

func editKey(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0]})
}

func editSpecial(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Mod: mod})
}

func editableTask() domain.Task {
	due := time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC)
	return domain.Task{UUID: "uuid", Description: "Original", Status: "pending", Project: "work", Priority: "H", Due: &due, DueRaw: "20260908T170000Z", Tags: []string{"one", "two"}, Recurrence: "weekly"}
}

func testEdit() EditModel {
	e := NewEdit(NewStyles(ResolveTheme("dark", true)), IconsFor("unicode"))
	e.SetSize(70, 20)
	e.SetCatalog([]string{"work", "personal"}, []string{"one", "two", "three"})
	e.OpenTask(editableTask(), FieldDescription)
	return e
}

func TestEditOpensWithDescriptionFocus(t *testing.T) {
	e := testEdit()
	if !e.Open || e.Focused != FieldDescription || !e.Input(FieldDescription).Focused() || e.Input(FieldDescription).Value() != "Original" {
		t.Fatalf("editor=%#v", e)
	}
}

func TestDirectOpenFocusesRequestedField(t *testing.T) {
	for _, field := range []EditField{FieldProject, FieldPriority, FieldDue, FieldScheduled, FieldTags} {
		e := NewEdit(Styles{}, Icons{})
		e.OpenTask(editableTask(), field)
		if e.Focused != field || !e.Input(field).Focused() {
			t.Errorf("field=%d focused=%d", field, e.Focused)
		}
	}
}

func TestDateFieldsOpenAsReadableLocalTimestampsWithoutChanges(t *testing.T) {
	e := NewEdit(Styles{}, Icons{})
	e.OpenTask(editableTask(), FieldDue)
	want := editableTask().Due.In(time.Local).Format("2006-01-02 15:04")
	if got := e.Input(FieldDue).Value(); got != want {
		t.Fatalf("due=%q want=%q", got, want)
	}
	if e.fieldChanged(FieldDue) {
		t.Fatal("display formatting must not create a date change")
	}
}

func TestDateFieldKeyboardStepsDayAndTime(t *testing.T) {
	e := NewEdit(Styles{}, Icons{})
	e.OpenTask(editableTask(), FieldDue)
	start := editableTask().Due.In(time.Local)

	e.Update(editSpecial(tea.KeyRight, tea.ModCtrl))
	wantDay := start.AddDate(0, 0, 1).Format("2006-01-02 15:04")
	if got := e.Input(FieldDue).Value(); got != wantDay {
		t.Fatalf("day step=%q want=%q", got, wantDay)
	}

	e.Update(editSpecial(tea.KeyUp, tea.ModCtrl))
	wantTime := start.AddDate(0, 0, 1).Add(30 * time.Minute).Format("2006-01-02 15:04")
	if got := e.Input(FieldDue).Value(); got != wantTime {
		t.Fatalf("time step=%q want=%q", got, wantTime)
	}
}

func TestDateFieldViewShowsKeyboardGuidance(t *testing.T) {
	e := testEdit()
	e.OpenTask(editableTask(), FieldDue)
	view := sanitizeComponentRender(e.View())
	for _, want := range []string{"YYYY-MM-DD HH:MM", "ctrl+←/→ day", "ctrl+↑/↓ 30m"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q: %q", want, view)
		}
	}
}

func TestTabAndShiftTabTraverseFields(t *testing.T) {
	e := testEdit()
	e.Update(editSpecial(tea.KeyTab, 0))
	if e.Focused != FieldProject {
		t.Fatalf("tab focused=%d", e.Focused)
	}
	e.Update(editSpecial(tea.KeyTab, tea.ModShift))
	if e.Focused != FieldDescription {
		t.Fatalf("shift tab focused=%d", e.Focused)
	}
}

func TestUpDownDoNotTraverseFieldsWithoutSuggestions(t *testing.T) {
	e := testEdit()
	e.SetCatalog(nil, nil)
	e.Update(editSpecial(tea.KeyDown, 0))
	e.Update(editSpecial(tea.KeyUp, 0))
	if e.Focused != FieldDescription {
		t.Fatalf("arrow keys changed focus to %d", e.Focused)
	}
}

func TestProjectSuggestionsOwnArrowsAndEnter(t *testing.T) {
	e := testEdit()
	e.Update(editKey("p")) // append to the existing project only after focus changes below
	// Re-open on project so the test is independent of Description's text.
	e.OpenTask(editableTask(), FieldProject)
	e.Inputs[FieldProject].SetValue("")
	e.Inputs[FieldProject].CursorEnd()
	e.refreshSuggestions()
	if !e.SuggestionsOpen || len(e.Suggestions) != 2 {
		t.Fatalf("suggestions=%#v", e.Suggestions)
	}
	e.Update(editSpecial(tea.KeyDown, 0))
	if e.SuggestionIndex != 1 {
		t.Fatalf("suggestion index=%d", e.SuggestionIndex)
	}
	want := e.Suggestions[e.SuggestionIndex]
	e.Update(editSpecial(tea.KeyEnter, 0))
	if e.Input(FieldProject).Value() != want {
		t.Fatalf("project value=%q want=%q", e.Input(FieldProject).Value(), want)
	}
	if e.SuggestionsOpen {
		t.Fatal("accepting a suggestion should close suggestions")
	}
}

func TestTabLeavesProjectWithoutAcceptingSuggestion(t *testing.T) {
	e := testEdit()
	e.OpenTask(editableTask(), FieldProject)
	e.Inputs[FieldProject].SetValue("")
	e.refreshSuggestions()

	e.Update(editSpecial(tea.KeyTab, 0))
	if e.Focused != FieldPriority {
		t.Fatalf("tab focused=%d want=%d", e.Focused, FieldPriority)
	}
	if e.Input(FieldProject).Value() != "" {
		t.Fatalf("tab accepted project suggestion %q", e.Input(FieldProject).Value())
	}
}

func TestDateFieldSuggestionsIncludeNextWeekAlias(t *testing.T) {
	e := testEdit()
	e.OpenTask(editableTask(), FieldScheduled)
	e.Inputs[FieldScheduled].SetValue("")
	e.refreshSuggestions()
	joined := "," + strings.Join(e.Suggestions, ",") + ","
	if !strings.Contains(joined, ",next-week,") {
		t.Fatalf("suggestions missing next-week alias: %#v", e.Suggestions)
	}
}

func TestPrioritySuggestionsUseFixedList(t *testing.T) {
	e := testEdit()
	e.OpenTask(editableTask(), FieldPriority)
	e.Inputs[FieldPriority].SetValue("")
	e.refreshSuggestions()
	if !strings.Contains(strings.Join(e.Suggestions, ","), "H") {
		t.Fatalf("suggestions=%#v", e.Suggestions)
	}
	e.Update(editSpecial(tea.KeyDown, 0))
	if e.SuggestionIndex != 1 {
		t.Fatalf("index=%d", e.SuggestionIndex)
	}
}

func TestCtrlSSavesOnlyValidatedMinimalDiff(t *testing.T) {
	e := testEdit()
	e.Inputs[FieldProject].SetValue("personal")
	e.Inputs[FieldProject].CursorEnd()
	_, cmd := e.Update(editSpecial('s', tea.ModCtrl))
	if cmd == nil {
		t.Fatal("ctrl+s did not return command")
	}
	message, ok := cmd().(EditSubmitMsg)
	if !ok || message.Diff.Project.Kind != domain.Set || message.Diff.Project.Value != "personal" || message.Diff.Description.Kind != domain.Unchanged {
		t.Fatalf("message=%#v", message)
	}
	e.ApplyMessage(message)
	if e.Open {
		t.Fatal("successful submit should close editor")
	}
}

func TestCtrlSRejectsEmptyDescriptionWithoutClosing(t *testing.T) {
	e := testEdit()
	e.Inputs[FieldDescription].SetValue("")
	_, cmd := e.Update(editSpecial('s', tea.ModCtrl))
	message, ok := cmd().(EditErrorMsg)
	if !ok {
		t.Fatalf("message=%T", cmd())
	}
	e.ApplyMessage(message)
	if !e.Open || e.Err == nil || e.Input(FieldDescription).Value() != "" {
		t.Fatalf("editor=%#v", e)
	}
}

func TestEscapeDiscardsChanges(t *testing.T) {
	e := testEdit()
	e.Inputs[FieldDescription].SetValue("changed")
	e.Update(editSpecial(tea.KeyEscape, 0))
	if e.Open || e.Task.Description != "Original" {
		t.Fatalf("editor=%#v", e)
	}
}

func TestEditChangedStateIncludesClear(t *testing.T) {
	e := testEdit()
	e.Inputs[FieldDue].SetValue("")
	if !e.fieldChanged(FieldDue) || e.CurrentSnapshot().Due != "" {
		t.Fatalf("editor=%#v", e)
	}
}

func TestEditTagValuesAreUniqueInSubmission(t *testing.T) {
	e := testEdit()
	e.Inputs[FieldTags].SetValue("one three three")
	message := e.submit()().(EditSubmitMsg)
	if len(message.After.Tags) != 2 || message.Diff.Tags.Changed == false || len(message.Diff.Tags.Add) != 1 || message.Diff.Tags.Add[0] != "three" {
		t.Fatalf("message=%#v", message)
	}
}

func TestEditViewShowsLabelsAndChangedMarker(t *testing.T) {
	e := testEdit()
	e.Inputs[FieldProject].SetValue("personal")
	view := sanitizeComponentRender(e.View())
	for _, want := range []string{"Edit task", "Description", "Project", "•", "was work", "1 changed"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q: %q", want, view)
		}
	}
}

func TestEditApplyMessageIgnoresUnrelatedMessages(t *testing.T) {
	e := testEdit()
	if message, handled := e.ApplyMessage(tea.WindowSizeMsg{Width: 1, Height: 1}); handled || message != nil {
		t.Fatalf("message=%#v handled=%v", message, handled)
	}
}
