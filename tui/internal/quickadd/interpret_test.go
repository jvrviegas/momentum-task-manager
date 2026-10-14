package quickadd

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInterpretNaturalDateTimeAndExplicitProject(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.FixedZone("Test", -4*60*60))
	got, err := Interpret("Prepare proposal tomorrow at 3pm #work", now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RequiresReview || !got.Valid || got.Task.Description != "Prepare proposal" || got.Task.Project != "work" || got.Task.Due != "20260922T150000" {
		t.Fatalf("interpretation=%#v", got)
	}
	if got.Timezone != "Test" {
		t.Fatalf("timezone=%q", got.Timezone)
	}
}

func TestInterpretWithLiteralsKeepsChosenPhraseAsText(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	got, err := InterpretWithLiterals("Read tomorrow by tomorrow", now, []Span{{Start: 5, End: 13}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Task.Description != "Read tomorrow by" || got.Task.Due != "20260922T000000" {
		t.Fatalf("task=%#v", got.Task)
	}
	got, err = InterpretWithLiterals("Read tomorrow", now, []Span{{Start: 5, End: 13}})
	if err != nil || got.RequiresReview || got.Task.Description != "Read tomorrow" || got.Task.Due != "" {
		t.Fatalf("interpretation=%#v err=%v", got, err)
	}
}

func TestInterpretRoadmapExampleConsumesOwnedCommaSeparators(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	got, err := Interpret("Prepare proposal tomorrow at 3pm, p1, every Friday, about 1h #work", now)
	if err != nil || got.Valid || got.Task.Project != "work" || got.Task.Priority != "H" || got.Task.Recurrence != "weekly" || got.Task.Estimate == nil || got.Task.Estimate.Minutes != 60 || got.Task.Due != "20260925T000000" || len(got.Diagnostics) == 0 {
		t.Fatalf("interpretation=%#v err=%v", got, err)
	}
}

func TestInterpretExplicitPrecedenceRetainsNaturalSource(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	got, err := Interpret("Write report tomorrow @2026-09-30 ~30m about 1h p1 !none", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Task.Due != "2026-09-30" || got.Task.Estimate == nil || got.Task.Estimate.Minutes != 30 || got.Task.Priority != "" {
		t.Fatalf("task=%#v", got.Task)
	}
	if got.Task.Description != "Write report tomorrow about 1h p1" {
		t.Fatalf("description=%q", got.Task.Description)
	}
	if len(got.Diagnostics) == 0 {
		t.Fatal("expected precedence diagnostics")
	}
}

func TestInterpretEffortPriorityAndRecurrence(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	got, err := Interpret("Review release every Friday at 3pm about an hour p2", now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Valid || got.Task.Description != "Review release" || got.Task.Recurrence != "weekly" || got.Task.Due != "20260925T150000" || got.Task.Estimate == nil || got.Task.Estimate.Minutes != 60 || got.Task.Priority != "M" {
		t.Fatalf("interpretation=%#v", got)
	}
}

func TestInterpretBareTimeAndUnsupportedThisFridayStayRecoverable(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	bare, err := Interpret("Call at 3pm", now)
	if err != nil || bare.Valid || bare.Task.Description != "Call at 3pm" {
		t.Fatalf("bare=%#v err=%v", bare, err)
	}
	unsupported, err := Interpret("Discuss this Friday", now)
	if err != nil || unsupported.Task.Description != "Discuss this Friday" {
		t.Fatalf("unsupported=%#v err=%v", unsupported, err)
	}
}

func TestInterpretDSTWallTimesRequireCorrection(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("timezone database unavailable")
	}
	gap, err := Interpret("Call tomorrow at 2:30am", time.Date(2026, 3, 7, 10, 0, 0, 0, location))
	if err != nil || gap.Valid || !strings.Contains(strings.ToLower(gap.Diagnostics[0].Message), "does not exist") {
		t.Fatalf("gap=%#v err=%v", gap, err)
	}
	fold, err := Interpret("Call tomorrow at 1:30am", time.Date(2026, 10, 31, 10, 0, 0, 0, location))
	if err != nil || fold.Valid || !strings.Contains(strings.ToLower(fold.Diagnostics[0].Message), "ambiguous") {
		t.Fatalf("fold=%#v err=%v", fold, err)
	}
}

func TestInterpretDeterministicReferenceTime(t *testing.T) {
	now := time.Date(2026, 2, 27, 23, 50, 0, 0, time.UTC)
	left, err := Interpret("Call tomorrow", now)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Interpret("Call tomorrow", now)
	if err != nil || left.Task.Due != right.Task.Due || left.Task.Description != right.Task.Description || !left.ReferenceTime.Equal(right.ReferenceTime) {
		t.Fatalf("left=%#v right=%#v err=%v", left, right, err)
	}
}

func TestInterpretRejectsMultipleInferredScalarValues(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		input string
		field quickaddFieldCheck
	}{
		{input: "Report p1 p2", field: quickaddFieldCheck{priority: true}},
		{input: "Report about 1h for 30 minutes", field: quickaddFieldCheck{estimate: true}},
		{input: "Report every day every Friday", field: quickaddFieldCheck{recurrence: true}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := Interpret(tc.input, now)
			if err != nil || got.Valid || got.Task.Description != tc.input || len(got.Diagnostics) == 0 {
				t.Fatalf("interpretation=%#v err=%v", got, err)
			}
			if tc.field.priority && got.Task.Priority != "" {
				t.Fatalf("priority=%q", got.Task.Priority)
			}
			if tc.field.estimate && got.Task.Estimate != nil {
				t.Fatalf("estimate=%#v", got.Task.Estimate)
			}
			if tc.field.recurrence && got.Task.Recurrence != "" {
				t.Fatalf("recurrence=%q", got.Task.Recurrence)
			}
		})
	}
}

type quickaddFieldCheck struct {
	priority, estimate, recurrence bool
}

func TestInterpretProtectsProseAndRecognizesInvalidTimeClauses(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		input       string
		valid       bool
		description string
	}{
		{input: "Email friday@example.com", valid: true, description: "Email friday@example.com"},
		{input: "Read https://example.com/tomorrow", valid: true, description: "Read https://example.com/tomorrow"},
		{input: "Review étape-p1", valid: true, description: "Review étape-p1"},
		{input: "Discuss every other Friday", valid: true, description: "Discuss every other Friday"},
		{input: "Call tomorrow at 25:00", valid: false, description: "Call at 25:00"},
		{input: "Call tomorrow at 3", valid: false, description: "Call at 3"},
		{input: "Call tomorrow at", valid: false, description: "Call at"},
		{input: "Report for 1.5 minutes", valid: false, description: "Report for 1.5 minutes"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := Interpret(tc.input, now)
			if err != nil || got.Valid != tc.valid || got.Task.Description != tc.description {
				t.Fatalf("interpretation=%#v err=%v", got, err)
			}
			if !tc.valid && len(got.Diagnostics) == 0 {
				t.Fatal("invalid candidate has no diagnostic")
			}
		})
	}
}

func FuzzInterpretIsDeterministicAndSpanSafe(f *testing.F) {
	f.Add("Email friday@example.com")
	f.Add("Prepare tomorrow at 3pm #work ~1h p1")
	f.Add("Discuss every other Friday")
	f.Fuzz(func(t *testing.T, input string) {
		now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
		left, leftErr := Interpret(input, now)
		right, rightErr := Interpret(input, now)
		if (leftErr == nil) != (rightErr == nil) {
			t.Fatalf("error instability: left=%v right=%v", leftErr, rightErr)
		}
		if leftErr != nil {
			return
		}
		if !reflect.DeepEqual(left, right) {
			t.Fatalf("nondeterministic interpretation: left=%#v right=%#v", left, right)
		}
		runes := []rune(input)
		for index, candidate := range left.Candidates {
			if candidate.Start < 0 || candidate.Start >= candidate.End || candidate.End > len(runes) {
				t.Fatalf("candidate %d has invalid span %#v for %q", index, candidate, input)
			}
			for otherIndex := index + 1; otherIndex < len(left.Candidates); otherIndex++ {
				other := left.Candidates[otherIndex]
				if candidate.Start < other.End && other.Start < candidate.End {
					t.Fatalf("overlapping candidate spans %#v and %#v", candidate, other)
				}
			}
		}
	})
}

func TestInterpretSupportsPluralRecurrenceIntervals(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		input       string
		recurrence  string
		description string
	}{
		{input: "Report every 2 week", recurrence: "2wks", description: "Report"},
		{input: "Report every 2 weeks", recurrence: "2wks", description: "Report"},
		{input: "Report every 2 day", recurrence: "2days", description: "Report"},
		{input: "Report every 2 days", recurrence: "2days", description: "Report"},
		{input: "Report every 2 weeks, #work", recurrence: "2wks", description: "Report"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := Interpret(tc.input, now)
			if err != nil || !got.Valid || !got.RequiresReview || got.Task.Recurrence != tc.recurrence || got.Task.Due != "20260921T000000" || got.Task.Description != tc.description {
				t.Fatalf("interpretation=%#v err=%v", got, err)
			}
		})
	}
}

func TestInterpretSupports24HourTimesBeforeFollowingTokens(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	for _, input := range []string{
		"Call tomorrow at 15:00",
		"Call tomorrow at 15:00 #work",
		"Call tomorrow at 15:00 p1",
		"Call at 15:00 tomorrow",
		"Call tomorrow   at   15:00, #work",
		"Call tomorrow at 3:00pm #work",
	} {
		t.Run(input, func(t *testing.T) {
			got, err := Interpret(input, now)
			if err != nil || !got.Valid || got.Task.Due != "20260922T150000" {
				t.Fatalf("interpretation=%#v err=%v", got, err)
			}
		})
	}
}

func TestInterpretRejectsAmbiguousWallTimesAcrossZones(t *testing.T) {
	cases := []struct {
		zone string
		date string
	}{
		{zone: "Europe/Lisbon", date: "2026-10-25 at 1:30am"},
		{zone: "Europe/Berlin", date: "2026-10-25 at 2:30am"},
		{zone: "Australia/Lord_Howe", date: "2026-04-05 at 1:45am"},
	}
	for _, tc := range cases {
		t.Run(tc.zone, func(t *testing.T) {
			location, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Skip("timezone database unavailable")
			}
			now := time.Date(2026, 9, 21, 10, 0, 0, 0, location)
			got, err := Interpret("Call "+tc.date, now)
			if err != nil || got.Valid || !strings.Contains(strings.ToLower(got.Diagnostics[0].Message), "ambiguous") {
				t.Fatalf("interpretation=%#v err=%v", got, err)
			}
		})
	}
}
