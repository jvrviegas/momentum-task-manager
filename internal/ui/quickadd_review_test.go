package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jvrviegas/momentum-task-manager/internal/quickadd"
)

func TestNaturalQuickAddSubmitsOnEnter(t *testing.T) {
	q := NewQuickAdd(NewStyles(ResolveTheme("dark", true)), IconsFor("unicode"))
	q.SetSize(90, 30)
	q.Now = time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	q.OpenQuickAdd("Prepare proposal tomorrow at 3pm #work")
	_, cmd := q.Update(specialQuickKey(tea.KeyEnter, 0))
	if cmd == nil {
		t.Fatal("enter did not schedule interpretation")
	}
	message := cmd()
	submission, ok := message.(QuickAddSubmitMsg)
	if !ok || submission.Task.Description != "Prepare proposal" || submission.Task.Project != "work" || submission.Task.Due != "20260922T150000" {
		t.Fatalf("message=%T %#v", message, message)
	}
	q.ApplyMessage(message)
	if q.ReviewOpen || q.Open {
		t.Fatalf("capture remained open: review=%v open=%v", q.ReviewOpen, q.Open)
	}
}

func TestInvalidNaturalQuickAddStillRequiresCorrection(t *testing.T) {
	for _, input := range []string{"Call at 3pm", "Call tomorrow at 3pm at 4pm", "Report for 0 minutes", "Report tomorrow every Friday"} {
		t.Run(input, func(t *testing.T) {
			q := NewQuickAdd(Styles{}, IconsFor("ascii"))
			q.SetSize(80, 24)
			q.Now = time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
			q.OpenQuickAdd(input)
			_, cmd := q.Update(specialQuickKey(tea.KeyEnter, 0))
			message := cmd()
			if _, ok := message.(QuickAddReviewMsg); !ok {
				t.Fatalf("invalid input bypassed correction: %#v", message)
			}
			q.ApplyMessage(message)
			if !q.Open || !q.ReviewOpen {
				t.Fatal("correction review did not remain open")
			}
		})
	}
}

func TestExplicitOnlyReviewPathSubmitsOriginalProse(t *testing.T) {
	q := reviewedQuickAdd(t, "Discuss Friday")
	_, cmd := q.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Mod: tea.ModCtrl}))
	if cmd == nil {
		t.Fatal("explicit-only did not submit")
	}
	result, ok := cmd().(QuickAddSubmitMsg)
	if !ok || result.Task.Description != "Discuss Friday" || result.Task.Due != "" {
		t.Fatalf("result=%#v", result)
	}
}

func TestCanceledReviewDoesNotSubmit(t *testing.T) {
	q := NewQuickAdd(Styles{}, IconsFor("ascii"))
	q.SetSize(80, 20)
	q.OpenQuickAdd("Call at 3pm")
	_, cmd := q.Update(specialQuickKey(tea.KeyEnter, 0))
	q.ApplyMessage(cmd())
	_, cmd = q.Update(tea.KeyPressMsg(tea.Key{Text: "esc", Code: tea.KeyEscape}))
	if q.ReviewOpen || !q.Open || q.Input.Value() != "Call at 3pm" {
		t.Fatalf("review=%v open=%v input=%q", q.ReviewOpen, q.Open, q.Input.Value())
	}
}

func reviewedQuickAdd(t *testing.T, input string) QuickAddModel {
	t.Helper()
	q := NewQuickAdd(Styles{}, IconsFor("ascii"))
	q.SetSize(90, 30)
	q.Now = time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	q.OpenQuickAdd(input)
	// Exercise review rendering/editing directly, independently of the Enter
	// fast path. Valid interpretations no longer open review on submission.
	interpretation, err := quickadd.Interpret(input, q.Now)
	if err != nil {
		t.Fatal(err)
	}
	q.openReview(interpretation)
	return q
}

func TestReviewXIsEditableAndCtrlXIsTheLiteralEscape(t *testing.T) {
	q := reviewedQuickAdd(t, "Fix x tomorrow")
	q.focusReviewField(quickAddReviewDescription)
	q.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	if !strings.HasSuffix(q.ReviewInputs[quickAddReviewDescription].Value(), "x") || !q.ReviewOpen {
		t.Fatalf("description=%q review=%v", q.ReviewInputs[quickAddReviewDescription].Value(), q.ReviewOpen)
	}

	q = reviewedQuickAdd(t, "Discuss Friday")
	_, cmd := q.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Mod: tea.ModCtrl}))
	if cmd == nil {
		t.Fatal("ctrl+x did not schedule explicit-only capture")
	}
	message, ok := cmd().(QuickAddSubmitMsg)
	if !ok || message.Task.Description != "Discuss Friday" || message.Task.Due != "" || !q.Open {
		t.Fatalf("message=%#v open=%v", message, q.Open)
	}
}

func TestReviewRequiresConflictCorrectionAndAcceptsEditedEstimate(t *testing.T) {
	q := reviewedQuickAdd(t, "Call tomorrow at 3pm at 4pm")
	_, cmd := q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if message, ok := cmd().(QuickAddErrorMsg); !ok || message.Err == nil {
		t.Fatalf("unresolved conflict message=%#v", message)
	}
	q.ReviewInputs[quickAddReviewDue].SetValue("20260922T150000")
	_, cmd = q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if message, ok := cmd().(QuickAddSubmitMsg); !ok || message.Task.Due != "20260922T150000" {
		t.Fatalf("corrected conflict message=%#v", message)
	}

	q = reviewedQuickAdd(t, "Report tomorrow every Friday")
	q.focusReviewField(quickAddReviewRecurrence)
	q.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	q.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	_, cmd = q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if message, ok := cmd().(QuickAddErrorMsg); !ok || message.Err == nil {
		t.Fatalf("type-delete recurrence was accepted: %#v", message)
	}

	q = reviewedQuickAdd(t, "Report tomorrow every Friday")
	q.ReviewInputs[quickAddReviewDue].SetValue("20260930T000000")
	q.ReviewInputs[quickAddReviewDue].SetValue("20260925T000000")
	q.ReviewTouched[quickAddReviewDue] = true
	_, cmd = q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if message, ok := cmd().(QuickAddErrorMsg); !ok || message.Err == nil {
		t.Fatalf("edit-restore anchor was accepted: %#v", message)
	}

	q.ReviewInputs[quickAddReviewDue].SetValue("20260930T000000")
	_, cmd = q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if message, ok := cmd().(QuickAddErrorMsg); !ok || message.Err == nil {
		t.Fatalf("mismatched anchor was accepted: %#v", message)
	}
	q.ReviewInputs[quickAddReviewDue].SetValue("20261002T000000")
	q.ReviewTouched[quickAddReviewDue] = true
	_, cmd = q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if message, ok := cmd().(QuickAddSubmitMsg); !ok || message.Task.Due != "20261002T000000" {
		t.Fatalf("corrected anchor message=%#v", message)
	}

	q = reviewedQuickAdd(t, "Report tomorrow every Friday")
	q.focusReviewField(quickAddReviewRecurrence)
	q.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Mod: tea.ModCtrl}))
	_, cmd = q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if message, ok := cmd().(QuickAddSubmitMsg); !ok || message.Task.Due != "20260925T000000" {
		t.Fatalf("explicitly resolved anchor message=%#v", message)
	}

	q = reviewedQuickAdd(t, "Report for 0 minutes")
	q.ReviewInputs[quickAddReviewEstimate].SetValue("30m")
	_, cmd = q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if message, ok := cmd().(QuickAddSubmitMsg); !ok || message.Task.Estimate == nil || message.Task.Estimate.Minutes != 30 {
		t.Fatalf("corrected estimate message=%#v", message)
	}

	q = reviewedQuickAdd(t, "Report for 0 minutes")
	q.ReviewInputs[quickAddReviewEstimate].SetValue("30m")
	q.ReviewInputs[quickAddReviewEstimate].SetValue("")
	q.ReviewTouched[quickAddReviewEstimate] = true
	_, cmd = q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if message, ok := cmd().(QuickAddSubmitMsg); !ok || message.Task.Estimate != nil {
		t.Fatalf("explicitly cleared estimate message=%#v", message)
	}
}

func TestReviewRendersTaskwarriorEquivalentNextOccurrence(t *testing.T) {
	q := reviewedQuickAdd(t, "Review every month")
	q.ReviewInputs[quickAddReviewDue].SetValue("20270131T150000")
	view := q.View()
	if !strings.Contains(view, "Next occurrence: 2027-02-28 15:00 UTC") {
		t.Fatalf("review has incorrect recurrence preview: %q", view)
	}
}

func TestShortReviewDiagnosticsCanBeScrolled(t *testing.T) {
	q := reviewedQuickAdd(t, "Call tomorrow at 3pm at 4pm")
	q.SetSize(28, 8)
	seen := false
	for index := 0; index < 8; index++ {
		if strings.Contains(q.View(), "time phrase") {
			seen = true
			break
		}
		q.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
	}
	if !seen {
		t.Fatalf("diagnostic tail was not accessible: %q", q.View())
	}
}

func TestFailedReviewedDraftCanBeRetriedWithoutAutomaticSubmission(t *testing.T) {
	q := reviewedQuickAdd(t, "Report for 0 minutes")
	q.ReviewInputs[quickAddReviewEstimate].SetValue("30m")
	_, cmd := q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	message, ok := cmd().(QuickAddSubmitMsg)
	if !ok {
		t.Fatalf("message=%T", message)
	}
	q.ApplyMessage(message)
	q.RestoreFailedTask(message.Task, message.Source, true, fmt.Errorf("hook failed"))
	if !q.Open || !q.ReviewOpen || !strings.Contains(q.View(), "hook failed") {
		t.Fatalf("failed review was not restored: open=%v review=%v view=%q", q.Open, q.ReviewOpen, q.View())
	}
	_, cmd = q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if retry, ok := cmd().(QuickAddSubmitMsg); !ok || retry.Task.Estimate == nil || retry.Task.Estimate.Minutes != 30 {
		t.Fatalf("restored retry=%#v", retry)
	}
}

func TestReviewRendersErrorsAndAllSubmittedFields(t *testing.T) {
	q := reviewedQuickAdd(t, "Call tomorrow #work +client >today")
	view := q.View()
	for _, want := range []string{"Project", "Scheduled", "Tags", "Timezone", "2026-09-22 00:00"} {
		if !strings.Contains(view, want) {
			t.Errorf("review missing %q: %q", want, view)
		}
	}
	q.focusReviewField(quickAddReviewScheduled)
	if !strings.Contains(q.View(), "Scheduled") {
		t.Fatalf("focused scheduled field is not visible: %q", q.View())
	}
	q.SetSize(28, 8)
	shortValues := map[int]string{
		quickAddReviewDescription: "Call",
		quickAddReviewDue:         "20260922T000000",
		quickAddReviewProject:     "work",
		quickAddReviewScheduled:   "today",
		quickAddReviewTags:        "client",
	}
	for field, label := range quickAddReviewFieldNames {
		q.focusReviewField(field)
		view := ansi.Strip(q.View())
		if !strings.Contains(view, label) || !strings.Contains(view, "ctrl+s") {
			t.Fatalf("short review hides field/action %q: %q", label, view)
		}
		// At 28 columns the focused cursor cell can push the first rune of a
		// 15-cell value out; the edit point (its tail) must stay visible.
		if want := shortValues[field]; want != "" && !strings.Contains(view, want[max(0, len(want)-14):]) {
			t.Fatalf("short review hides value %q for %q: %q", want, label, view)
		}
	}
	q.ParseErr = fmt.Errorf("hook failed")
	if !strings.Contains(q.View(), "hook failed") {
		t.Fatalf("review error hidden: %q", q.View())
	}
}

func TestReviewConfirmsMixedEstimateUnchanged(t *testing.T) {
	q := reviewedQuickAdd(t, "Write report tomorrow ~90m")
	_, cmd := q.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	if cmd == nil {
		t.Fatalf("confirm blocked: err=%v", q.ParseErr)
	}
	message, ok := cmd().(QuickAddSubmitMsg)
	if !ok || message.Task.Estimate == nil || message.Task.Estimate.Minutes != 90 {
		t.Fatalf("message=%#v err=%v", cmd(), q.ParseErr)
	}
}
