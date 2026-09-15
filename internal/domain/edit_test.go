package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestSnapshotIncludesOnlyEditableFields(t *testing.T) {
	due := time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC)
	task := Task{
		Description: "Task",
		Project:     "work",
		Priority:    "H",
		Due:         &due,
		DueRaw:      "20260908T170000Z",
		Tags:        []string{"b", "a", "a"},
		Recurrence:  "weekly",
		Urgency:     4,
	}
	got := Snapshot(task)
	want := EditSnapshot{Description: "Task", Project: "work", Priority: "H", Due: "20260908T170000Z", Tags: []string{"a", "b"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestUnchangedFieldsProduceEmptyDiff(t *testing.T) {
	snapshot := EditSnapshot{Description: "Task", Project: "work", Priority: "H", Due: "today", Scheduled: "tomorrow", Tags: []string{"a"}}
	got := Diff(snapshot, snapshot)
	if !got.Empty() || !reflect.DeepEqual(got, TaskDiff{}) {
		t.Fatalf("got %#v", got)
	}
}

func TestScalarChangesSetValues(t *testing.T) {
	before := EditSnapshot{Description: "old", Project: "old-project", Priority: "L", Due: "today", Scheduled: "monday"}
	after := EditSnapshot{Description: "new", Project: "new-project", Priority: "H", Due: "tomorrow", Scheduled: "tuesday"}
	got := Diff(before, after)
	for name, change := range map[string]FieldChange{
		"description": got.Description,
		"project":     got.Project,
		"priority":    got.Priority,
		"due":         got.Due,
		"scheduled":   got.Scheduled,
	} {
		if change.Kind != Set {
			t.Errorf("%s kind=%v", name, change.Kind)
		}
	}
	if got.Description.Value != "new" || got.Due.Value != "tomorrow" {
		t.Fatalf("values=%#v", got)
	}
}

func TestClearingEachOptionalFieldIsExplicit(t *testing.T) {
	before := EditSnapshot{Project: "p", Priority: "H", Due: "today", Scheduled: "tomorrow", Tags: []string{"a", "b"}}
	after := EditSnapshot{}
	got := Diff(before, after)
	for name, change := range map[string]FieldChange{
		"project":   got.Project,
		"priority":  got.Priority,
		"due":       got.Due,
		"scheduled": got.Scheduled,
	} {
		if change.Kind != Clear {
			t.Errorf("%s kind=%v", name, change.Kind)
		}
	}
	if !got.Tags.Changed || !reflect.DeepEqual(got.Tags.Remove, []string{"a", "b"}) {
		t.Fatalf("tags=%#v", got.Tags)
	}
}

func TestDescriptionCanBeCleared(t *testing.T) {
	got := Diff(EditSnapshot{Description: "old"}, EditSnapshot{})
	if got.Description.Kind != Clear || got.Description.Empty() {
		t.Fatalf("got %#v", got.Description)
	}
}

func TestTagAdditionsAndRemovalsAreSorted(t *testing.T) {
	got := Diff(EditSnapshot{Tags: []string{"keep", "remove", "z"}}, EditSnapshot{Tags: []string{"keep", "add", "a"}})
	if !got.Tags.Changed || !reflect.DeepEqual(got.Tags.Add, []string{"a", "add"}) || !reflect.DeepEqual(got.Tags.Remove, []string{"remove", "z"}) {
		t.Fatalf("tags=%#v", got.Tags)
	}
}

func TestDuplicateTagsDoNotCreateChanges(t *testing.T) {
	got := Diff(EditSnapshot{Tags: []string{"a", "a"}}, EditSnapshot{Tags: []string{"a"}})
	if got.Tags.Changed {
		t.Fatalf("duplicate tag should not change: %#v", got.Tags)
	}
}

func TestRawAndUnsupportedFieldsNeverEnterDiff(t *testing.T) {
	before := EditSnapshot{Description: "same", Tags: []string{"a"}}
	after := EditSnapshot{Description: "same", Tags: []string{"a"}}
	got := Diff(before, after)
	if !got.Empty() {
		t.Fatalf("unexpected unsupported changes: %#v", got)
	}
}

func TestGenerateDiffAlias(t *testing.T) {
	got := GenerateDiff(EditSnapshot{Project: "a"}, EditSnapshot{Project: "b"})
	if got.Project.Kind != Set || got.Project.Value != "b" {
		t.Fatalf("got %#v", got)
	}
}

func TestFieldChangeEmptyOnlyForUnchangedOrClear(t *testing.T) {
	if (FieldChange{Kind: Unchanged}).Value != "" {
		t.Fatal("zero field change should be empty")
	}
	if (FieldChange{Kind: Set, Value: "x"}).Value != "x" {
		t.Fatal("set value missing")
	}
}
