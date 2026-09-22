package domain

import (
	"testing"
	"time"
)

func TestParseRecurrencePresetsAndIntervals(t *testing.T) {
	for input, want := range map[string]string{
		"daily": "daily", "weekdays": "weekdays", "weekly": "weekly", "biweekly": "2wks",
		"monthly": "monthly", "2 weeks": "2wks", "3 months": "3mo",
	} {
		got, err := ParseRecurrence(input)
		if err != nil || got != want {
			t.Errorf("ParseRecurrence(%q)=%q err=%v want %q", input, got, err, want)
		}
	}
}

func TestParseRecurrencePhraseUsesAnchor(t *testing.T) {
	now := time.Date(2026, 9, 21, 13, 0, 0, 0, time.UTC)
	got, err := ParseRecurrencePhrase("every Friday", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Expression != "weekly" || got.Anchor.Weekday() != time.Friday || got.Anchor.Day() != 25 {
		t.Fatalf("definition=%#v", got)
	}
}

func TestRecurrenceTargetDistinguishesGeneratedInstance(t *testing.T) {
	task := Task{UUID: "instance", Parent: "template", Status: "pending", Recurrence: "daily"}
	if !task.IsRecurrenceInstance() || task.IsRecurrenceTemplate() || task.RecurrenceTargetUUID() != "template" {
		t.Fatalf("task=%#v", task)
	}
}
