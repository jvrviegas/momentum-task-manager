package taskwarrior

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
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

func TestAddArgsTranslatesNextWeekToFridayOfNextCalendarWeek(t *testing.T) {
	got, err := AddArgs(domain.NewTask{Description: "Plan", Due: "next-week", Scheduled: "next-week"})
	want := []string{"add", "Plan", "due:sow+1w+4d", "scheduled:sow+1w+4d"}
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

func TestAddArgsSerializesEstimateAsWholeMinutes(t *testing.T) {
	estimate := domain.Estimate{Minutes: 90}
	got, err := AddArgs(domain.NewTask{Description: "task", Estimate: &estimate})
	want := []string{"add", "task", "estimate:90min"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v err=%v want %#v", got, err, want)
	}
	if _, err := AddArgs(domain.NewTask{Description: "task", Estimate: &domain.Estimate{Minutes: 0}}); err == nil {
		t.Fatal("invalid estimate was accepted")
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

func TestModifyArgsTranslatesNextWeekToFridayOfNextCalendarWeek(t *testing.T) {
	setNextWeek := domain.FieldChange{Kind: domain.Set, Value: "next-week"}
	got, err := ModifyArgs("uuid", domain.TaskDiff{Due: setNextWeek, Scheduled: setNextWeek})
	want := []string{"uuid", "modify", "due:sow+1w+4d", "scheduled:sow+1w+4d"}
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

func TestModifyArgsSerializesEstimateSetAndClear(t *testing.T) {
	estimate := &domain.Estimate{Minutes: 45}
	set, err := ModifyArgs("uuid", domain.TaskDiff{Estimate: domain.EstimateChange{Kind: domain.Set, Value: estimate}})
	if err != nil || !reflect.DeepEqual(set, []string{"uuid", "modify", "estimate:45min"}) {
		t.Fatalf("set=%#v err=%v", set, err)
	}
	clear, err := ModifyArgs("uuid", domain.TaskDiff{Estimate: domain.EstimateChange{Kind: domain.Clear}})
	if err != nil || !reflect.DeepEqual(clear, []string{"uuid", "modify", "estimate:"}) {
		t.Fatalf("clear=%#v err=%v", clear, err)
	}
	for _, arg := range set {
		if arg == "estimate:45m" {
			t.Fatal("ambiguous Taskwarrior minute suffix was emitted")
		}
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

func TestOrdinaryAddSkipsEstimateUDAReadiness(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0}}}}
	client := NewClientWithRunner("task", runner)
	if err := client.Add(context.Background(), domain.NewTask{Description: "ordinary task"}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"task", "add", "ordinary task"}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls=%#v want=%#v", runner.calls, want)
	}
}

func TestEstimateMutationChecksUDAImmediatelyBeforeAdd(t *testing.T) {
	estimate := domain.Estimate{Minutes: 60}
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 0, Stdout: "duration"}},
		{result: CommandResult{ExitCode: 0}},
	}}
	client := NewClientWithRunner("task", runner)
	if err := client.Add(context.Background(), domain.NewTask{Description: "task", Estimate: &estimate}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"task", "_get", "rc.uda.estimate.type"},
		{"task", "add", "task", "estimate:60min"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls=%#v want=%#v", runner.calls, want)
	}
}

func TestEstimateMutationRefusesWrongUDAAndReadinessFailure(t *testing.T) {
	estimate := domain.Estimate{Minutes: 60}
	wrongRunner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0, Stdout: "string"}}}}
	wrongErr := NewClientWithRunner("task", wrongRunner).Add(context.Background(), domain.NewTask{Description: "task", Estimate: &estimate})
	var wrongUDAErr *EstimateUDAError
	if !errors.As(wrongErr, &wrongUDAErr) || wrongUDAErr.State != EstimateUDAWrongType || len(wrongRunner.calls) != 1 {
		t.Fatalf("wrong err=%T %v calls=%#v", wrongErr, wrongErr, wrongRunner.calls)
	}

	failureRunner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 9, Stderr: "probe failed"}, err: errors.New("exit status 9")}}}
	failureErr := NewClientWithRunner("task", failureRunner).Modify(context.Background(), "uuid", domain.TaskDiff{Estimate: domain.EstimateChange{Kind: domain.Set, Value: &estimate}})
	var unavailableErr *EstimateUDAError
	if !errors.As(failureErr, &unavailableErr) || unavailableErr.State != EstimateUDAUnavailable || len(failureRunner.calls) != 1 {
		t.Fatalf("failure err=%T %v calls=%#v", failureErr, failureErr, failureRunner.calls)
	}

	mutationRunner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 0, Stdout: "duration"}},
		{result: CommandResult{ExitCode: 7, Stderr: "hook rejected"}, err: errors.New("exit status 7")},
	}}
	mutationErr := NewClientWithRunner("task", mutationRunner).Add(context.Background(), domain.NewTask{Description: "task", Estimate: &estimate})
	var commandErr *CommandError
	if !errors.As(mutationErr, &commandErr) || !strings.Contains(mutationErr.Error(), "hook rejected") || len(mutationRunner.calls) != 2 {
		t.Fatalf("mutation err=%T %v calls=%#v", mutationErr, mutationErr, mutationRunner.calls)
	}
}

func TestEstimateMutationRefusesBeforeAddWhenUDAIsMissing(t *testing.T) {
	estimate := domain.Estimate{Minutes: 60}
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0}}}}
	client := NewClientWithRunner("task", runner)
	err := client.Add(context.Background(), domain.NewTask{Description: "task", Estimate: &estimate})
	var readinessErr *EstimateUDAError
	if !errors.As(err, &readinessErr) || readinessErr.State != EstimateUDAMissing {
		t.Fatalf("err=%T %v", err, err)
	}
	if len(runner.calls) != 1 || !reflect.DeepEqual(runner.calls[0], []string{"task", "_get", "rc.uda.estimate.type"}) {
		t.Fatalf("calls=%#v", runner.calls)
	}
	if !strings.Contains(err.Error(), "uda.estimate.type=duration") {
		t.Fatalf("error lacks setup guidance: %v", err)
	}
}

func TestEstimateModifyChecksUDAForSetAndClearButNotOrdinaryChanges(t *testing.T) {
	estimate := &domain.Estimate{Minutes: 90}
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 0, Stdout: "duration"}},
		{result: CommandResult{ExitCode: 0}},
		{result: CommandResult{ExitCode: 0, Stdout: "duration"}},
		{result: CommandResult{ExitCode: 0}},
		{result: CommandResult{ExitCode: 0}},
	}}
	client := NewClientWithRunner("task", runner)
	if err := client.Modify(context.Background(), "uuid", domain.TaskDiff{Estimate: domain.EstimateChange{Kind: domain.Set, Value: estimate}}); err != nil {
		t.Fatal(err)
	}
	if err := client.Modify(context.Background(), "uuid", domain.TaskDiff{Estimate: domain.EstimateChange{Kind: domain.Clear}}); err != nil {
		t.Fatal(err)
	}
	if err := client.Modify(context.Background(), "uuid", domain.TaskDiff{Project: domain.FieldChange{Kind: domain.Set, Value: "work"}}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"task", "_get", "rc.uda.estimate.type"},
		{"task", "uuid", "modify", "estimate:90min"},
		{"task", "_get", "rc.uda.estimate.type"},
		{"task", "uuid", "modify", "estimate:"},
		{"task", "uuid", "modify", "project:work"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls=%#v want=%#v", runner.calls, want)
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
