package taskwarrior

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jvrviegas/momentum/internal/domain"
)

// ProjectMigrationScope is the active Taskwarrior context captured for one
// preview and reused for its export, guarded writes, and reconciliation.
type ProjectMigrationScope struct {
	ContextName string
	ReadFilter  string
}

// ProjectMigrationExport contains one context-scoped pending snapshot.
type ProjectMigrationExport struct {
	Scope ProjectMigrationScope
	Tasks []domain.Task
}

// ProjectTaskOutcomeKind classifies one guarded UUID operation after
// reconciliation, rather than trusting the process exit code alone.
type ProjectTaskOutcomeKind string

const (
	ProjectTaskChanged   ProjectTaskOutcomeKind = "changed"
	ProjectTaskSkipped   ProjectTaskOutcomeKind = "skipped"
	ProjectTaskFailed    ProjectTaskOutcomeKind = "failed"
	ProjectTaskAmbiguous ProjectTaskOutcomeKind = "ambiguous"
)

// ProjectTaskOutcome reports the command and the observed post-command task.
type ProjectTaskOutcome struct {
	Mapping  domain.ProjectTaskMapping
	Kind     ProjectTaskOutcomeKind
	Command  CommandResult
	Observed *domain.Task
	Err      error
}

// ExportPendingInContext reads pending, non-recurring tasks in the active
// Taskwarrior context. Taskwarrior's export intentionally ignores context, so
// the captured read filter is passed explicitly when one exists.
func (c *CommandClient) ExportPendingInContext(ctx context.Context) (ProjectMigrationExport, error) {
	scope, err := c.projectMigrationScope(ctx)
	if err != nil {
		return ProjectMigrationExport{}, err
	}
	tasks, err := c.exportTasks(ctx, scope, true, "")
	if err != nil {
		return ProjectMigrationExport{}, err
	}
	return ProjectMigrationExport{Scope: scope, Tasks: tasks}, nil
}

// ProjectMigrationScope captures the active context without changing it.
func (c *CommandClient) ProjectMigrationScope(ctx context.Context) (ProjectMigrationScope, error) {
	return c.projectMigrationScope(ctx)
}

// ProjectMigrationModifyArgs builds the one-UUID pending/exact-project guard.
// It never constructs a shell command.
func ProjectMigrationModifyArgs(scope ProjectMigrationScope, mapping domain.ProjectTaskMapping) ([]string, error) {
	if err := validateUUID(mapping.UUID); err != nil {
		return nil, err
	}
	if mapping.OldValue == "" || mapping.NewValue == "" {
		return nil, errors.New("project migration values are required")
	}
	args := []string{mapping.UUID}
	if filter := contextFilterArg(scope.ReadFilter); filter != "" {
		args = append(args, filter)
	}
	args = append(args,
		"status:pending",
		"recur.none:",
		"project.is:"+mapping.OldValue,
		"modify",
		"project:"+mapping.NewValue,
	)
	if err := ValidateArgs(args); err != nil {
		return nil, err
	}
	return args, nil
}

// ProjectMigrationExportArgs builds the explicit pending export for a scope.
func ProjectMigrationExportArgs(scope ProjectMigrationScope) ([]string, error) {
	return projectMigrationExportArgs(scope, true, "")
}

// ProjectMigrationTaskExportArgs builds a UUID export used for reconciliation.
func ProjectMigrationTaskExportArgs(scope ProjectMigrationScope, uuid string) ([]string, error) {
	if err := validateUUID(uuid); err != nil {
		return nil, err
	}
	return projectMigrationExportArgs(scope, false, uuid)
}

// ApplyProjectTask runs one guarded project assignment and re-exports that UUID
// in the captured scope. A successful process is reported as changed only when
// the saved task is observed with the requested destination value.
func (c *CommandClient) ApplyProjectTask(ctx context.Context, scope ProjectMigrationScope, mapping domain.ProjectTaskMapping) ProjectTaskOutcome {
	outcome := ProjectTaskOutcome{Mapping: mapping}
	args, err := ProjectMigrationModifyArgs(scope, mapping)
	if err != nil {
		outcome.Kind = ProjectTaskFailed
		outcome.Err = err
		return outcome
	}
	result, commandErr := c.run(ctx, "project-modify", args...)
	outcome.Command = result

	observed, reconcileErr := c.exportTask(ctx, scope, mapping.UUID)
	if reconcileErr != nil {
		outcome.Kind = ProjectTaskAmbiguous
		outcome.Err = joinMigrationErrors(commandErr, reconcileErr)
		return outcome
	}
	outcome.Observed = observed
	if observed == nil {
		if isNoTaskResult(commandErr) {
			outcome.Kind = ProjectTaskSkipped
			return outcome
		}
		outcome.Kind = ProjectTaskAmbiguous
		outcome.Err = joinMigrationErrors(commandErr, errors.New("Taskwarrior did not return the migrated task"))
		return outcome
	}

	switch {
	case observed.Project == mapping.NewValue:
		outcome.Kind = ProjectTaskChanged
	case observed.Project == mapping.OldValue && commandErr != nil:
		if isNoTaskResult(commandErr) {
			outcome.Kind = ProjectTaskSkipped
		} else if isAmbiguousCommandError(commandErr) {
			outcome.Kind = ProjectTaskAmbiguous
			outcome.Err = commandErr
		} else {
			outcome.Kind = ProjectTaskFailed
			outcome.Err = commandErr
		}
	case observed.Project == mapping.OldValue:
		outcome.Kind = ProjectTaskAmbiguous
		outcome.Err = errors.New("Taskwarrior reported success but the project value did not change")
	default:
		outcome.Kind = ProjectTaskFailed
		outcome.Err = joinMigrationErrors(commandErr, fmt.Errorf("reconciled project value is %q, want %q", observed.Project, mapping.NewValue))
	}
	return outcome
}

func (c *CommandClient) projectMigrationScope(ctx context.Context) (ProjectMigrationScope, error) {
	name, err := c.Context(ctx)
	if err != nil {
		return ProjectMigrationScope{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ProjectMigrationScope{}, nil
	}
	key := "rc.context." + name + ".read"
	if err := ValidateArgs([]string{"_get", key}); err != nil {
		return ProjectMigrationScope{}, err
	}
	result, err := c.run(ctx, "context-filter", "_get", key)
	if err != nil {
		return ProjectMigrationScope{}, err
	}
	filter := strings.TrimSpace(result.Stdout)
	if err := ValidateArgs([]string{contextFilterArg(filter)}); err != nil {
		return ProjectMigrationScope{}, err
	}
	return ProjectMigrationScope{ContextName: name, ReadFilter: filter}, nil
}

func (c *CommandClient) exportTasks(ctx context.Context, scope ProjectMigrationScope, pending bool, uuid string) ([]domain.Task, error) {
	var args []string
	var err error
	if pending {
		args, err = projectMigrationExportArgs(scope, true, "")
	} else {
		args, err = ProjectMigrationTaskExportArgs(scope, uuid)
	}
	if err != nil {
		return nil, err
	}
	result, err := c.run(ctx, "project-export", args...)
	if err != nil {
		return nil, err
	}
	return decodeMigrationTasks(result.Stdout)
}

func (c *CommandClient) exportTask(ctx context.Context, scope ProjectMigrationScope, uuid string) (*domain.Task, error) {
	tasks, err := c.exportTasks(ctx, scope, false, uuid)
	if err != nil {
		var commandErr *CommandError
		if errors.As(err, &commandErr) && commandErr.ExitCode == 1 && strings.Contains(strings.ToLower(commandErr.Stderr), "no tasks specified") {
			return nil, nil
		}
		return nil, err
	}
	for i := range tasks {
		if tasks[i].UUID == uuid {
			return &tasks[i], nil
		}
	}
	if scope.ReadFilter == "" {
		return nil, nil
	}
	// A successful rename can move the task outside the captured read filter.
	// Reconcile the already-confirmed UUID without a broad filter; this does
	// not add a task to the plan or widen the mutation command.
	tasks, err = c.exportTasks(ctx, ProjectMigrationScope{}, false, uuid)
	if err != nil {
		var commandErr *CommandError
		if errors.As(err, &commandErr) && commandErr.ExitCode == 1 && strings.Contains(strings.ToLower(commandErr.Stderr), "no tasks specified") {
			return nil, nil
		}
		return nil, err
	}
	for i := range tasks {
		if tasks[i].UUID == uuid {
			return &tasks[i], nil
		}
	}
	return nil, nil
}

func decodeMigrationTasks(output string) ([]domain.Task, error) {
	if strings.TrimSpace(output) == "" {
		return []domain.Task{}, nil
	}
	var tasks []domain.Task
	if err := json.Unmarshal([]byte(output), &tasks); err != nil {
		return nil, fmt.Errorf("decode Taskwarrior project export: %w", err)
	}
	return tasks, nil
}

func projectMigrationExportArgs(scope ProjectMigrationScope, pending bool, uuid string) ([]string, error) {
	args := make([]string, 0, 5)
	if uuid != "" {
		if err := validateUUID(uuid); err != nil {
			return nil, err
		}
		args = append(args, uuid)
	}
	if filter := contextFilterArg(scope.ReadFilter); filter != "" {
		args = append(args, filter)
	}
	if pending {
		args = append(args, "status:pending", "recur.none:")
	}
	args = append(args, "export")
	if err := ValidateArgs(args); err != nil {
		return nil, err
	}
	return args, nil
}

func contextFilterArg(filter string) string {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return ""
	}
	return "(" + filter + ")"
}

func isAmbiguousCommandError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func isNoTaskResult(err error) bool {
	if err == nil {
		return false
	}
	var commandErr *CommandError
	return errors.As(err, &commandErr) && commandErr.ExitCode == 1 && strings.Contains(strings.ToLower(commandErr.Stderr), "no tasks specified")
}

func joinMigrationErrors(left, right error) error {
	switch {
	case left == nil:
		return right
	case right == nil:
		return left
	default:
		return errors.Join(left, right)
	}
}
