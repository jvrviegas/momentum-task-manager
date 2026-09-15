package taskwarrior

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum/internal/domain"
)

func TestAddArgsTranslatesEveryQuickAddField(t *testing.T) {
	got, err := AddArgs(domain.NewTask{
		Description: "Prepare proposal",
		Project:     "work.client",
		Priority:    "H",
		Due:         "tomorrow",
		Scheduled:   "monday",
		Tags:        []string{"planning", "client"},
	})
	want := []string{"add", "Prepare proposal", "project:work.client", "due:tomorrow", "scheduled:monday", "+client", "+planning", "priority:H"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v err=%v, want %#v", got, err, want)
	}
}

func TestAddArgsKeepsDescriptionAsOneArgument(t *testing.T) {
	got, err := AddArgs(domain.NewTask{Description: "email bob@example.com"})
	if err != nil || !reflect.DeepEqual(got, []string{"add", "email bob@example.com"}) {
		t.Fatalf("got %#v err=%v", got, err)
	}
}

func TestAddArgsOmitsNonePriorityAndEmptyOptionalFields(t *testing.T) {
	got, err := AddArgs(domain.NewTask{Description: "task", Priority: ""})
	if err != nil || !reflect.DeepEqual(got, []string{"add", "task"}) {
		t.Fatalf("got %#v err=%v", got, err)
	}
}

func TestAddArgsRejectsEmptyDescription(t *testing.T) {
	if _, err := AddArgs(domain.NewTask{Description: "  "}); err == nil {
		t.Fatal("expected empty description error")
	}
}

func TestModifyArgsUsesUUIDAndMinimalChanges(t *testing.T) {
	diff := domain.TaskDiff{
		Project: domain.FieldChange{Kind: domain.Set, Value: "work.client"},
		Due:     domain.FieldChange{Kind: domain.Clear},
		Tags:    domain.TagChange{Changed: true, Add: []string{"new"}, Remove: []string{"old"}},
	}
	got, err := ModifyArgs("uuid-1", diff)
	want := []string{"uuid-1", "modify", "project:work.client", "due:", "-old", "+new"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v err=%v want %#v", got, err, want)
	}
}

func TestModifyArgsIncludesAllScalarClearForms(t *testing.T) {
	clear := domain.FieldChange{Kind: domain.Clear}
	got, err := ModifyArgs("uuid", domain.TaskDiff{Description: clear, Project: clear, Priority: clear, Due: clear, Scheduled: clear})
	want := []string{"uuid", "modify", "description:", "project:", "priority:", "due:", "scheduled:"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v err=%v want %#v", got, err, want)
	}
}

func TestModifyArgsRejectsEmptyDiff(t *testing.T) {
	_, err := ModifyArgs("uuid", domain.TaskDiff{})
	if !errors.Is(err, ErrEmptyDiff) {
		t.Fatalf("got %v", err)
	}
}

func TestModifyArgsRejectsUnsafeUUID(t *testing.T) {
	for _, uuid := range []string{"", " uuid", "uuid\n", "-uuid"} {
		if _, err := ModifyArgs(uuid, domain.TaskDiff{Project: domain.FieldChange{Kind: domain.Set, Value: "p"}}); !errors.Is(err, ErrInvalidUUID) {
			t.Errorf("%q: got %v", uuid, err)
		}
	}
}

func TestMutationMethodsUseExpectedUUIDArgv(t *testing.T) {
	runner := &fakeRunner{responses: make([]fakeResponse, 7)}
	for i := range runner.responses {
		runner.responses[i].result.ExitCode = 0
	}
	client := NewClientWithRunner("task", runner)
	ctx := context.Background()
	if err := client.Complete(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if err := client.Delete(ctx, "two"); err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx, "three"); err != nil {
		t.Fatal(err)
	}
	if err := client.Stop(ctx, "four"); err != nil {
		t.Fatal(err)
	}
	if err := client.Undo(ctx); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"task", "one", "done"},
		{"task", "two", "delete", "rc.confirmation=no"},
		{"task", "three", "start"},
		{"task", "four", "stop"},
		{"task", "undo"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls=%#v want=%#v", runner.calls, want)
	}
}

func TestMutationErrorPropagatesHookFailure(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 7, Stderr: "hook failed"}, err: errors.New("exit status 7")}}}
	err := NewClientWithRunner("task", runner).Complete(context.Background(), "uuid")
	var commandErr *CommandError
	if !errors.As(err, &commandErr) || !strings.Contains(err.Error(), "hook failed") {
		t.Fatalf("got %T %v", err, err)
	}
}

func TestValidateArgsRejectsControlCharacters(t *testing.T) {
	if err := ValidateArgs([]string{"add", "bad\x00value"}); err == nil {
		t.Fatal("expected control-character error")
	}
	if err := ValidateArgs([]string{"add", "safe"}); err != nil {
		t.Fatal(err)
	}
}
