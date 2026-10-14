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

func TestRecurrenceNextMatchesTaskwarriorCalendarSemantics(t *testing.T) {
	lisbon, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		anchor     time.Time
		expression string
		want       time.Time
	}{
		{
			name:       "daily preserves instant across DST",
			anchor:     time.Date(2027, 3, 27, 15, 0, 0, 0, lisbon),
			expression: "daily",
			want:       time.Date(2027, 3, 28, 16, 0, 0, 0, lisbon),
		},
		{
			name:       "weekdays skip weekend and preserve instant",
			anchor:     time.Date(2027, 3, 26, 15, 0, 0, 0, lisbon),
			expression: "weekdays",
			want:       time.Date(2027, 3, 29, 16, 0, 0, 0, lisbon),
		},
		{
			name:       "weekly preserves instant across DST",
			anchor:     time.Date(2027, 3, 27, 15, 0, 0, 0, lisbon),
			expression: "weekly",
			want:       time.Date(2027, 4, 3, 16, 0, 0, 0, lisbon),
		},
		{
			name:       "monthly preserves wall clock",
			anchor:     time.Date(2027, 3, 27, 15, 0, 0, 0, lisbon),
			expression: "monthly",
			want:       time.Date(2027, 4, 27, 15, 0, 0, 0, lisbon),
		},
		{
			name:       "monthly clamps month end",
			anchor:     time.Date(2027, 1, 31, 15, 0, 0, 0, lisbon),
			expression: "monthly",
			want:       time.Date(2027, 2, 28, 15, 0, 0, 0, lisbon),
		},
		{
			name:       "monthly clamps leap year month end",
			anchor:     time.Date(2028, 1, 31, 15, 0, 0, 0, lisbon),
			expression: "monthly",
			want:       time.Date(2028, 2, 29, 15, 0, 0, 0, lisbon),
		},
		{
			name:       "month interval is thirty day duration",
			anchor:     time.Date(2026, 12, 31, 15, 0, 0, 0, lisbon),
			expression: "2mo",
			want:       time.Date(2027, 3, 1, 15, 0, 0, 0, lisbon),
		},
		{
			name:       "quarterly is ninety day duration",
			anchor:     time.Date(2027, 1, 31, 15, 0, 0, 0, lisbon),
			expression: "quarterly",
			want:       time.Date(2027, 5, 1, 16, 0, 0, 0, lisbon),
		},
		{
			name:       "annual clamps leap day",
			anchor:     time.Date(2028, 2, 29, 15, 0, 0, 0, lisbon),
			expression: "annual",
			want:       time.Date(2029, 2, 28, 15, 0, 0, 0, lisbon),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := RecurrenceNext(test.anchor, test.expression)
			if !ok || !got.Equal(test.want) || got.Location() != test.want.Location() {
				t.Fatalf("RecurrenceNext(%v, %q)=%v, %v; want %v", test.anchor, test.expression, got, ok, test.want)
			}
		})
	}
}

func TestRecurrenceTargetDistinguishesGeneratedInstance(t *testing.T) {
	task := Task{UUID: "instance", Parent: "template", Status: "pending", Recurrence: "daily"}
	if !task.IsRecurrenceInstance() || task.IsRecurrenceTemplate() || task.RecurrenceTargetUUID() != "template" {
		t.Fatalf("task=%#v", task)
	}
}
