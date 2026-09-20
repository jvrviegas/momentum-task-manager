package taskwarrior

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeResponse struct {
	result CommandResult
	err    error
}

type fakeRunner struct {
	calls     [][]string
	responses []fakeResponse
}

func (f *fakeRunner) Run(_ context.Context, command string, args ...string) (CommandResult, error) {
	f.calls = append(f.calls, append([]string{command}, args...))
	if len(f.responses) == 0 {
		return CommandResult{ExitCode: 0}, nil
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response.result, response.err
}

const exportFixture = `[{"id":1,"uuid":"one","description":"First","status":"pending","project":"work","tags":["a"],"urgency":4.2},{"id":2,"uuid":"two","description":"Second","status":"pending","tags":["b"],"urgency":2.1}]`

func TestExportPendingUsesExpectedArgvAndDecodes(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0, Stdout: exportFixture}}}}
	client := NewClientWithRunner("task-test", runner)
	got, err := client.ExportPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].UUID != "one" || got[1].Project != "" {
		t.Fatalf("unexpected tasks: %#v", got)
	}
	wantCall := []string{"task-test", "status:pending", "export"}
	if !reflect.DeepEqual(runner.calls[0], wantCall) {
		t.Fatalf("argv=%#v want=%#v", runner.calls[0], wantCall)
	}
}

func TestExportCompletedUsesContextStatusAndExclusiveDateBoundary(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 0, Stdout: "work\n"}},
		{result: CommandResult{ExitCode: 0, Stdout: "project:work\n"}},
		{result: CommandResult{ExitCode: 0, Stdout: `[]`}},
	}}
	client := NewClientWithRunner("task-test", runner)
	_, err := client.ExportCompleted(context.Background(), time.Date(2026, 8, 9, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := [][]string{
		{"task-test", "_get", "rc.context"},
		{"task-test", "_get", "rc.context.work.read"},
		{"task-test", "(project:work)", "status:completed", "end.after:2026-08-09", "export"},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls=%#v want=%#v", runner.calls, wantCalls)
	}
}

func TestExportPendingEmptyOutputReturnsEmptySlice(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0}}}}
	got, err := NewClientWithRunner("task", runner).ExportPending(context.Background())
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v err=%v", got, err)
	}
}

func TestExportPendingReturnsParseError(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0, Stdout: "not json"}}}}
	_, err := NewClientWithRunner("task", runner).ExportPending(context.Background())
	if err == nil || !strings.Contains(err.Error(), "decode Taskwarrior export") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestCommandFailureRetainsRedactedStderrAndExitMetadata(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{
		ExitCode: 42,
		Stderr:   "encryption_secret=super-secret hook rejected",
		Duration: 3 * time.Millisecond,
	}, err: errors.New("exit status 42")}}}
	_, err := NewClientWithRunner("task", runner).ExportPending(context.Background())
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("expected CommandError, got %T %v", err, err)
	}
	if commandErr.ExitCode != 42 || commandErr.Duration != 3*time.Millisecond || strings.Contains(commandErr.Stderr, "super-secret") || !strings.Contains(commandErr.Stderr, "REDACTED") {
		t.Fatalf("unexpected command error: %#v", commandErr)
	}
}

func TestContextReadsWithoutChangingTaskwarriorContext(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0, Stdout: "work\n"}}}}
	got, err := NewClientWithRunner("task", runner).Context(context.Background())
	if err != nil || got != "work" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if !reflect.DeepEqual(runner.calls[0], []string{"task", "_get", "rc.context"}) {
		t.Fatalf("argv=%#v", runner.calls[0])
	}
}

func TestProjectsCombinesScriptOutputAndExport(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 0, Stdout: "zeta\nalpha\n"}},
		{result: CommandResult{ExitCode: 0, Stdout: exportFixture}},
	}}
	got, err := NewClientWithRunner("task", runner).Projects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"alpha", "work", "zeta"}) {
		t.Fatalf("projects=%#v", got)
	}
	if len(runner.calls) != 2 || !reflect.DeepEqual(runner.calls[0], []string{"task", "_projects"}) {
		t.Fatalf("calls=%#v", runner.calls)
	}
}

func TestProjectsFallsBackToExportWhenProjectScriptFails(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{
		{result: CommandResult{ExitCode: 1, Stderr: "unsupported"}, err: errors.New("unsupported")},
		{result: CommandResult{ExitCode: 0, Stdout: exportFixture}},
	}}
	got, err := NewClientWithRunner("task", runner).Projects(context.Background())
	if err != nil || !reflect.DeepEqual(got, []string{"work"}) {
		t.Fatalf("projects=%#v err=%v", got, err)
	}
}

func TestTagsOnlyReturnsExportedUserTags(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0, Stdout: exportFixture}}}}
	got, err := NewClientWithRunner("task", runner).Tags(context.Background())
	if err != nil || !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("tags=%#v err=%v", got, err)
	}
}

func TestRunnerReceivesContextDeadline(t *testing.T) {
	var sawDeadline bool
	runner := RunnerFunc(func(ctx context.Context, _ string, _ ...string) (CommandResult, error) {
		_, sawDeadline = ctx.Deadline()
		return CommandResult{ExitCode: 0, Stdout: "[]"}, nil
	})
	client := NewClientWithRunner("task", runner)
	client.Timeout = time.Second
	if _, err := client.ExportPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !sawDeadline {
		t.Fatal("adapter did not apply a finite command deadline")
	}
}

func TestRedactCoversURLCredentials(t *testing.T) {
	got := Redact("url https://user:password@example.test/path")
	if strings.Contains(got, "user:password") || !strings.Contains(got, "REDACTED") {
		t.Fatalf("got %q", got)
	}
}
