package taskwarrior

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/jvrviegas/momentum/internal/domain"
)

const defaultTimeout = 15 * time.Second

// Client is the application-facing Taskwarrior adapter.
type Client interface {
	ExportPending(ctx context.Context) ([]domain.Task, error)
	Add(ctx context.Context, input domain.NewTask) error
	Modify(ctx context.Context, uuid string, diff domain.TaskDiff) error
	Complete(ctx context.Context, uuid string) error
	Delete(ctx context.Context, uuid string) error
	Start(ctx context.Context, uuid string) error
	Stop(ctx context.Context, uuid string) error
	Undo(ctx context.Context) error
	Sync(ctx context.Context) (SyncResult, error)
	Projects(ctx context.Context) ([]string, error)
}

// SyncResult reports a completed Taskwarrior sync.
type SyncResult struct {
	Changed bool
	Output  string
}

// CommandResult is the captured result of one Taskwarrior process.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// Runner is injectable so adapter tests never need the user's Taskwarrior.
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

// CommandClient implements Client using a Taskwarrior executable.
type CommandClient struct {
	Binary    string
	Runner    Runner
	Timeout   time.Duration
	OnCommand func(kind string, result CommandResult, err error)
}

// NewClient constructs a real Taskwarrior adapter.
func NewClient(binary string) *CommandClient {
	if binary == "" {
		binary = "task"
	}
	return &CommandClient{Binary: binary, Runner: ExecRunner{}, Timeout: defaultTimeout}
}

// NewClientWithRunner constructs an adapter for unit tests or an alternate
// process supervisor.
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
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	result, err := c.Runner.Run(commandCtx, c.Binary, args...)
	if c.OnCommand != nil {
		c.OnCommand(kind, result, err)
	}
	if err != nil || result.ExitCode != 0 {
		return result, &CommandError{
			Kind:     kind,
			Args:     append([]string(nil), args...),
			ExitCode: result.ExitCode,
			Stderr:   Redact(result.Stderr),
			Duration: result.Duration,
			Cause:    err,
		}
	}
	return result, nil
}

// Version reads Taskwarrior's version without touching task data.
func (c *CommandClient) Version(ctx context.Context) (string, error) {
	result, err := c.run(ctx, "version", "--version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

// ExportPending reads one context-respecting, machine-readable export.
func (c *CommandClient) ExportPending(ctx context.Context) ([]domain.Task, error) {
	result, err := c.run(ctx, "export", "status:pending", "export")
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

// Context returns the active Taskwarrior context without changing it.
func (c *CommandClient) Context(ctx context.Context) (string, error) {
	result, err := c.run(ctx, "context", "_get", "rc.context")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

// Projects combines script-oriented project output with projects in the
// current export. A failing _projects command is tolerated if export succeeds.
func (c *CommandClient) Projects(ctx context.Context) ([]string, error) {
	result, projectErr := c.run(ctx, "projects", "_projects")
	values := make(map[string]struct{})
	for _, line := range strings.Split(result.Stdout, "\n") {
		if value := strings.TrimSpace(line); value != "" {
			values[value] = struct{}{}
		}
	}

	tasks, exportErr := c.ExportPending(ctx)
	if exportErr == nil {
		for _, task := range tasks {
			if task.Project != "" {
				values[task.Project] = struct{}{}
			}
		}
	}
	projects := make([]string, 0, len(values))
	for value := range values {
		projects = append(projects, value)
	}
	sort.Strings(projects)
	if projectErr != nil && exportErr != nil {
		return nil, fmt.Errorf("discover projects: %w (export: %v)", projectErr, exportErr)
	}
	return projects, nil
}

// Tags returns actual user tags found in pending export data. Taskwarrior
// virtual tags are not included because they are not present in task.tags.
func (c *CommandClient) Tags(ctx context.Context) ([]string, error) {
	tasks, err := c.ExportPending(ctx)
	if err != nil {
		return nil, err
	}
	values := make(map[string]struct{})
	for _, task := range tasks {
		for _, tag := range task.Tags {
			if tag != "" {
				values[tag] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}
