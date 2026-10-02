package taskwarrior

import (
	"reflect"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func TestAddArgsIncludesValidatedRecurrenceAndAnchor(t *testing.T) {
	args, err := AddArgs(domain.NewTask{Description: "weekly review", Due: "friday", Recurrence: "weekly"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"add", "weekly review", "due:friday", "recur:weekly"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%#v want=%#v", args, want)
	}
}

func TestStopRecurrenceArgsExpiresTemplateWithoutClearingHistory(t *testing.T) {
	args, err := StopRecurrenceArgs("uuid")
	if err != nil || !reflect.DeepEqual(args, []string{"uuid", "modify", "until:today", "rc.recurrence.confirmation=no"}) {
		t.Fatalf("args=%#v err=%v", args, err)
	}
}

func TestModifyArgsCanSetAndClearRecurrence(t *testing.T) {
	args, err := ModifyArgs("uuid", domain.TaskDiff{Recurrence: domain.FieldChange{Kind: domain.Set, Value: "2 weeks"}})
	if err != nil || !reflect.DeepEqual(args, []string{"uuid", "modify", "recur:2wks"}) {
		t.Fatalf("set args=%#v err=%v", args, err)
	}
	args, err = ModifyArgs("uuid", domain.TaskDiff{Recurrence: domain.FieldChange{Kind: domain.Clear}})
	if err != nil || !reflect.DeepEqual(args, []string{"uuid", "modify", "recur:"}) {
		t.Fatalf("clear args=%#v err=%v", args, err)
	}
}
