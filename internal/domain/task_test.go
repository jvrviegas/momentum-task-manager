package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTaskUnmarshalRetainsOptionalDatesAndRawFields(t *testing.T) {
	data := []byte(`{
		"id": 7,
		"uuid": "abc-123",
		"description": "Prepare proposal",
		"status": "pending",
		"project": "work.client",
		"priority": "H",
		"due": "20260908T170000Z",
		"scheduled": null,
		"tags": ["planning", "client"],
		"depends": ["other"],
		"annotations": [{"description":"Ask finance","entry":"20260907T100000Z"}],
		"recur": "weekly",
		"urgency": 12.5,
		"custom_uda": "untouched"
	}`)

	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if task.UUID != "abc-123" || task.ID != 7 || task.Description != "Prepare proposal" {
		t.Fatalf("unexpected identity: %+v", task)
	}
	if task.Due == nil || task.Due.Location() != time.UTC {
		t.Fatalf("expected UTC due date, got %#v", task.Due)
	}
	if task.Scheduled != nil {
		t.Fatal("null scheduled date should remain absent")
	}
	if len(task.Tags) != 2 || task.Tags[0] != "planning" {
		t.Fatalf("unexpected tags: %#v", task.Tags)
	}
	if len(task.Annotations) != 1 || task.Annotations[0].Entry == nil {
		t.Fatalf("unexpected annotations: %#v", task.Annotations)
	}
	if got, ok := task.Raw("custom_uda"); !ok || string(got) != `"untouched"` {
		t.Fatalf("raw custom field not retained: %s, %v", got, ok)
	}
}

func TestTaskUnmarshalAbsentAndZeroLikeDatesAreDistinguishable(t *testing.T) {
	var absent Task
	if err := json.Unmarshal([]byte(`{"uuid":"a","status":"pending"}`), &absent); err != nil {
		t.Fatal(err)
	}
	if absent.Due != nil || absent.DueRaw != "" {
		t.Fatalf("absent due should be nil: %#v", absent)
	}

	var present Task
	if err := json.Unmarshal([]byte(`{"uuid":"b","status":"pending","due":"20060101T000000Z"}`), &present); err != nil {
		t.Fatal(err)
	}
	if present.Due == nil || present.DueRaw == "" {
		t.Fatalf("present due should be distinguishable: %#v", present)
	}
}

func TestParseTaskwarriorTimeFixtures(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
		isUTC bool
	}{
		{name: "compact utc", value: "20260908T170000Z", want: "2026-09-08T17:00:00Z", isUTC: true},
		{name: "rfc3339", value: "2026-09-08T17:00:00-04:00", want: "2026-09-08T17:00:00-04:00"},
		{name: "date", value: "2026-09-08", want: "2026-09-08T00:00:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTaskwarriorTime(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			format := "2006-01-02T15:04:05Z07:00"
			if tc.name == "date" {
				format = "2006-01-02T15:04:05"
			}
			if got.Format(format) != tc.want {
				t.Fatalf("got %s, want %s", got.Format(format), tc.want)
			}
			if tc.isUTC && got.Location() != time.UTC {
				t.Fatalf("expected UTC location, got %v", got.Location())
			}
		})
	}
}

func TestTaskUnmarshalRejectsInvalidDate(t *testing.T) {
	var task Task
	err := json.Unmarshal([]byte(`{"uuid":"bad","due":"not-a-date"}`), &task)
	if err == nil || !strings.Contains(err.Error(), "due") {
		t.Fatalf("expected due parse error, got %v", err)
	}
}

func TestTaskIsPendingIsCaseInsensitive(t *testing.T) {
	if !(Task{Status: "PENDING"}).IsPending() {
		t.Fatal("PENDING should be pending")
	}
	if (Task{Status: "completed"}).IsPending() {
		t.Fatal("completed should not be pending")
	}
}

func TestDateInUsesRequestedLocation(t *testing.T) {
	value := time.Date(2026, 9, 8, 23, 30, 0, 0, time.UTC)
	got, ok := DateIn(&value, time.FixedZone("west", -5*60*60))
	if !ok || got.Day() != 8 || got.Hour() != 18 {
		t.Fatalf("unexpected local conversion: %v, %v", got, ok)
	}
}
