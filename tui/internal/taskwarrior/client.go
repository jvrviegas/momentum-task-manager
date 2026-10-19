package taskwarrior

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/diagnostics"
)

const defaultTimeout = 15 * time.Second

// CommandResult is the captured result of one Taskwarrior process.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// Runner is injectable so tests never need the user's Taskwarrior.
type Runner interface {
	Run(ctx context.Context, command string, args ...string) (CommandResult, error)
}

// RunnerFunc adapts a function into a Runner.
type RunnerFunc func(context.Context, string, ...string) (CommandResult, error)

func (f RunnerFunc) Run(ctx context.Context, command string, args ...string) (CommandResult, error) {
	return f(ctx, command, args...)
}

// ExecRunner invokes an executable directly with argv. It never starts a shell.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, command string, args ...string) (CommandResult, error) {
	started := time.Now()
	cmd := exec.CommandContext(ctx, command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: -1,
		Duration: time.Since(started),
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	return result, err
}

// CommandClient reads tasks with a Taskwarrior executable.
type CommandClient struct {
	Binary  string
	Runner  Runner
	Timeout time.Duration
}

// NewClient constructs a reader that runs binary.
func NewClient(binary string) *CommandClient {
	if binary == "" {
		binary = "task"
	}
	return &CommandClient{Binary: binary, Runner: ExecRunner{}, Timeout: defaultTimeout}
}

// NewClientWithRunner constructs a reader for unit tests.
func NewClientWithRunner(binary string, runner Runner) *CommandClient {
	client := NewClient(binary)
	if runner != nil {
		client.Runner = runner
	}
	return client
}

func (c *CommandClient) run(ctx context.Context, kind string, args ...string) (CommandResult, error) {
	if c == nil || c.Runner == nil {
		return CommandResult{}, &CommandError{Kind: kind, ExitCode: -1, Cause: errors.New("Taskwarrior runner is not configured")}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := c.Runner.Run(commandCtx, c.Binary, args...)
	if err != nil || result.ExitCode != 0 {
		return result, &CommandError{
			Kind:     kind,
			Args:     append([]string(nil), args...),
			ExitCode: result.ExitCode,
			Stderr:   diagnostics.Redact(result.Stderr),
			Duration: result.Duration,
			Cause:    err,
		}
	}
	return result, nil
}

// ExportAll reads every task, whatever its status, for an import, and leaves
// Taskwarrior's data as it was. Taskwarrior 3.5 garbage-collects before an
// export, which writes and creates the instances a series is due, so garbage
// collection is off for the command; recurrence and hooks are off too, in
// case another version runs them on their own.
func (c *CommandClient) ExportAll(ctx context.Context) ([]domain.Task, error) {
	return c.exportFilteredTasks(ctx, "rc.recurrence=off", "rc.gc=off", "rc.hooks=off")
}

func (c *CommandClient) exportFilteredTasks(ctx context.Context, filters ...string) ([]domain.Task, error) {
	args := append(append([]string(nil), filters...), "export")
	result, err := c.run(ctx, "export", args...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(result.Stdout) == "" {
		return []domain.Task{}, nil
	}
	var tasks []domain.Task
	if err := json.Unmarshal([]byte(result.Stdout), &tasks); err != nil {
		return nil, fmt.Errorf("decode Taskwarrior export: %w", err)
	}
	return tasks, nil
}
