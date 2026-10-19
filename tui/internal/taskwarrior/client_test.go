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

func TestExportAllUsesExpectedArgvAndDecodes(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0, Stdout: exportFixture}}}}
	client := NewClientWithRunner("task-test", runner)
	got, err := client.ExportAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].UUID != "one" || got[1].Project != "" {
		t.Fatalf("unexpected tasks: %#v", got)
	}
	wantCall := []string{"task-test", "rc.recurrence=off", "rc.gc=off", "rc.hooks=off", "export"}
	if !reflect.DeepEqual(runner.calls[0], wantCall) {
		t.Fatalf("argv=%#v want=%#v", runner.calls[0], wantCall)
	}
}

func TestExportAllEmptyOutputReturnsEmptySlice(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0}}}}
	got, err := NewClientWithRunner("task", runner).ExportAll(context.Background())
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v err=%v", got, err)
	}
}

func TestExportAllReturnsParseError(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{result: CommandResult{ExitCode: 0, Stdout: "not json"}}}}
	_, err := NewClientWithRunner("task", runner).ExportAll(context.Background())
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
	_, err := NewClientWithRunner("task", runner).ExportAll(context.Background())
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("expected CommandError, got %T %v", err, err)
	}
	if commandErr.ExitCode != 42 || commandErr.Duration != 3*time.Millisecond || strings.Contains(commandErr.Stderr, "super-secret") || !strings.Contains(commandErr.Stderr, "REDACTED") {
		t.Fatalf("unexpected command error: %#v", commandErr)
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
	if _, err := client.ExportAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !sawDeadline {
		t.Fatal("reader did not apply a finite command deadline")
	}
}
