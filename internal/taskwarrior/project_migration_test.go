package taskwarrior

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

const migrationExportFixture = `[{"uuid":"one","description":"One","status":"pending","project":"work"},{"uuid":"two","description":"Two","status":"pending","project":"work.client"}]`

func TestExportPendingInContextCapturesScopeAndUsesExplicitFilter(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 0, Stdout: "work\n"}},
		{result: CommandResult{ExitCode: 0, Stdout: "project:work or +urgent\n"}},
		{result: CommandResult{ExitCode: 0, Stdout: migrationExportFixture}},
	}}
	client := NewClientWithRunner("task-test", runner)
	got, err := client.ExportPendingInContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Scope != (ProjectMigrationScope{ContextName: "work", ReadFilter: "project:work or +urgent"}) || len(got.Tasks) != 2 {
		t.Fatalf("export=%#v", got)
	}
	wantCalls := [][]string{
		{"task-test", "_get", "rc.context"},
		{"task-test", "_get", "rc.context.work.read"},
		{"task-test", "(project:work or +urgent)", "status:pending", "recur.none:", "export"},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls=%#v want=%#v", runner.calls, wantCalls)
	}
}

func TestExportPendingInContextWithoutActiveContextAddsNoFilter(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 0}},
		{result: CommandResult{ExitCode: 0, Stdout: migrationExportFixture}},
	}}
	got, err := NewClientWithRunner("task", runner).ExportPendingInContext(context.Background())
	if err != nil || got.Scope != (ProjectMigrationScope{}) || len(got.Tasks) != 2 {
		t.Fatalf("export=%#v err=%v", got, err)
	}
	if !reflect.DeepEqual(runner.calls[1], []string{"task", "status:pending", "recur.none:", "export"}) {
		t.Fatalf("export argv=%#v", runner.calls)
	}
}

func TestApplyProjectTaskUsesGuardedUUIDArgvAndReconcilesChangedTask(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 0}},
		{result: CommandResult{ExitCode: 0, Stdout: `[{"uuid":"one","status":"pending","project":"delivery"}]`}},
	}}
	client := NewClientWithRunner("task", runner)
	scope := ProjectMigrationScope{ContextName: "work", ReadFilter: "project:work"}
	mapping := domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"}
	outcome := client.ApplyProjectTask(context.Background(), scope, mapping)
	if outcome.Kind != ProjectTaskChanged || outcome.Observed == nil || outcome.Observed.Project != "delivery" || outcome.Err != nil {
		t.Fatalf("outcome=%#v", outcome)
	}
	want := [][]string{
		{"task", "one", "(project:work)", "status:pending", "recur.none:", "project.is:work", "modify", "project:delivery"},
		{"task", "one", "(project:work)", "export"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls=%#v want=%#v", runner.calls, want)
	}
}

func TestApplyProjectTaskClassifiesStaleEligibilityAsSkipped(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 1, Stderr: "No tasks specified."}, err: errors.New("exit status 1")},
		{result: CommandResult{ExitCode: 0, Stdout: `[{"uuid":"one","status":"completed","project":"work"}]`}},
	}}
	outcome := NewClientWithRunner("task", runner).ApplyProjectTask(context.Background(), ProjectMigrationScope{}, domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"})
	if outcome.Kind != ProjectTaskSkipped || outcome.Observed == nil || outcome.Observed.Status != "completed" {
		t.Fatalf("outcome=%#v", outcome)
	}
}

func TestApplyProjectTaskClassifiesTimeoutAsAmbiguousAfterReconciliation(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: -1}, err: context.DeadlineExceeded},
		{result: CommandResult{ExitCode: 0, Stdout: `[{"uuid":"one","status":"pending","project":"work"}]`}},
	}}
	outcome := NewClientWithRunner("task", runner).ApplyProjectTask(context.Background(), ProjectMigrationScope{}, domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"})
	if outcome.Kind != ProjectTaskAmbiguous || outcome.Observed == nil || outcome.Err == nil || !errors.Is(outcome.Err, context.DeadlineExceeded) {
		t.Fatalf("outcome=%#v", outcome)
	}
}

func TestApplyProjectTaskClassifiesHookRejectionAsFailedAfterReconciliation(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 7, Stderr: "hook rejected"}, err: errors.New("exit status 7")},
		{result: CommandResult{ExitCode: 0, Stdout: `[{"uuid":"one","status":"pending","project":"work"}]`}},
	}}
	outcome := NewClientWithRunner("task", runner).ApplyProjectTask(context.Background(), ProjectMigrationScope{}, domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"})
	if outcome.Kind != ProjectTaskFailed || outcome.Observed == nil || outcome.Observed.Project != "work" || outcome.Err == nil || !strings.Contains(outcome.Err.Error(), "hook rejected") {
		t.Fatalf("outcome=%#v", outcome)
	}
}

func TestApplyProjectTaskClassifiesUnreconciledResultAsAmbiguous(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 0}},
		{result: CommandResult{ExitCode: 0, Stdout: "not json"}},
	}}
	outcome := NewClientWithRunner("task", runner).ApplyProjectTask(context.Background(), ProjectMigrationScope{}, domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"})
	if outcome.Kind != ProjectTaskAmbiguous || outcome.Err == nil {
		t.Fatalf("outcome=%#v", outcome)
	}
}

func TestApplyProjectTaskRejectsUnsafeMappingWithoutRunningCommand(t *testing.T) {
	runner := &fakeRunner{}
	outcome := NewClientWithRunner("task", runner).ApplyProjectTask(context.Background(), ProjectMigrationScope{}, domain.ProjectTaskMapping{UUID: "bad uuid", OldValue: "work", NewValue: "delivery"})
	if outcome.Kind != ProjectTaskFailed || outcome.Err == nil || len(runner.calls) != 0 {
		t.Fatalf("outcome=%#v calls=%#v", outcome, runner.calls)
	}
}
