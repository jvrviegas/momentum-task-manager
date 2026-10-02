package taskwarrior

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

var (
	ErrInvalidUUID = errors.New("Taskwarrior UUID is required and must not contain whitespace")
	ErrEmptyDiff   = errors.New("task edit contains no changed supported fields")
)

// AddArgs builds the exact argv for a safe Taskwarrior add operation.
func AddArgs(input domain.NewTask) ([]string, error) {
	if strings.TrimSpace(input.Description) == "" {
		return nil, errors.New("task description is required")
	}
	args := []string{"add", input.Description}
	if input.Project != "" {
		args = append(args, "project:"+input.Project)
	}
	if input.Due != "" {
		args = append(args, "due:"+normalizeDateAlias(input.Due))
	}
	if input.Scheduled != "" {
		args = append(args, "scheduled:"+normalizeDateAlias(input.Scheduled))
	}
	if input.Recurrence != "" {
		recurrence, err := domain.ParseRecurrence(input.Recurrence)
		if err != nil {
			return nil, err
		}
		if input.Due == "" {
			return nil, errors.New("recurring tasks require a due date anchor")
		}
		args = append(args, "recur:"+recurrence)
	}
	if err := validateNewEstimate(input.Estimate); err != nil {
		return nil, err
	}
	if input.Estimate != nil {
		args = append(args, "estimate:"+input.Estimate.TaskwarriorValue())
	}
	tags := append([]string(nil), input.Tags...)
	sort.Strings(tags)
	for _, tag := range tags {
		if tag != "" {
			args = append(args, "+"+tag)
		}
	}
	if input.Priority != "" {
		args = append(args, "priority:"+input.Priority)
	}
	return args, nil
}

// ModifyArgs builds UUID-based argv for a minimal supported-field edit.
func ModifyArgs(uuid string, diff domain.TaskDiff) ([]string, error) {
	if err := validateUUID(uuid); err != nil {
		return nil, err
	}
	if diff.Empty() {
		return nil, ErrEmptyDiff
	}
	args := []string{uuid, "modify"}
	appendChange := func(name string, change domain.FieldChange) {
		switch change.Kind {
		case domain.Set:
			args = append(args, name+":"+change.Value)
		case domain.Clear:
			args = append(args, name+":")
		}
	}
	appendChange("description", diff.Description)
	appendChange("project", diff.Project)
	appendChange("priority", diff.Priority)
	diff.Due.Value = normalizeDateAlias(diff.Due.Value)
	diff.Scheduled.Value = normalizeDateAlias(diff.Scheduled.Value)
	appendChange("due", diff.Due)
	appendChange("scheduled", diff.Scheduled)
	if diff.Recurrence.Kind == domain.Set {
		recurrence, err := domain.ParseRecurrence(diff.Recurrence.Value)
		if err != nil {
			return nil, err
		}
		args = append(args, "recur:"+recurrence)
	} else if diff.Recurrence.Kind == domain.Clear {
		args = append(args, "recur:")
	}
	if err := validateEstimateChange(diff.Estimate); err != nil {
		return nil, err
	}
	switch diff.Estimate.Kind {
	case domain.Set:
		args = append(args, "estimate:"+diff.Estimate.Value.TaskwarriorValue())
	case domain.Clear:
		args = append(args, "estimate:")
	case domain.Unchanged:
	default:
		return nil, errors.New("invalid estimate change kind")
	}
	if diff.Tags.Changed {
		removals := append([]string(nil), diff.Tags.Remove...)
		additions := append([]string(nil), diff.Tags.Add...)
		sort.Strings(removals)
		sort.Strings(additions)
		for _, tag := range removals {
			if tag != "" {
				args = append(args, "-"+tag)
			}
		}
		for _, tag := range additions {
			if tag != "" {
				args = append(args, "+"+tag)
			}
		}
	}
	return args, nil
}

func normalizeDateAlias(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "next-week") {
		return "sow+1w+4d"
	}
	return value
}

func validateUUID(uuid string) error {
	if uuid == "" || strings.TrimSpace(uuid) != uuid || strings.ContainsAny(uuid, "\t\r\n ") || strings.HasPrefix(uuid, "-") {
		return ErrInvalidUUID
	}
	return nil
}

func simpleUUIDArgs(uuid, action string) ([]string, error) {
	if err := validateUUID(uuid); err != nil {
		return nil, err
	}
	return []string{uuid, action}, nil
}

// Add invokes Taskwarrior after building argv from the parsed task.
func (c *CommandClient) Add(ctx context.Context, input domain.NewTask) error {
	args, err := AddArgs(input)
	if err != nil {
		return err
	}
	if input.Estimate != nil {
		if _, err := c.EstimateUDAReadiness(ctx); err != nil {
			return err
		}
	}
	_, err = c.run(ctx, "add", args...)
	return err
}

// Modify invokes one UUID-based modify command.
func (c *CommandClient) Modify(ctx context.Context, uuid string, diff domain.TaskDiff) error {
	args, err := ModifyArgs(uuid, diff)
	if err != nil {
		return err
	}
	if diff.Estimate.Kind != domain.Unchanged {
		if _, err := c.EstimateUDAReadiness(ctx); err != nil {
			return err
		}
	}
	if diff.Recurrence.Kind != domain.Unchanged {
		args = append(args, "rc.recurrence.confirmation=no")
	}
	_, err = c.run(ctx, "modify", args...)
	return err
}

// Complete marks one task done by UUID.
func (c *CommandClient) Complete(ctx context.Context, uuid string) error {
	args, err := simpleUUIDArgs(uuid, "done")
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "complete", args...)
	return err
}

// Delete deletes one task after Momentum has already confirmed interactively.
func (c *CommandClient) Delete(ctx context.Context, uuid string) error {
	args, err := simpleUUIDArgs(uuid, "delete")
	if err != nil {
		return err
	}
	args = append(args, "rc.confirmation=no")
	_, err = c.run(ctx, "delete", args...)
	return err
}

// Start starts one task by UUID.
func (c *CommandClient) Start(ctx context.Context, uuid string) error {
	args, err := simpleUUIDArgs(uuid, "start")
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "start", args...)
	return err
}

// Stop stops one task by UUID.
func (c *CommandClient) Stop(ctx context.Context, uuid string) error {
	args, err := simpleUUIDArgs(uuid, "stop")
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "stop", args...)
	return err
}

// StopRecurrenceArgs targets the recurrence template and expires it before
// the next occurrence. Clearing recur directly is rejected by Taskwarrior 3.x
// for templates; until preserves existing generated/completed history.
func StopRecurrenceArgs(uuid string) ([]string, error) {
	if err := validateUUID(uuid); err != nil {
		return nil, err
	}
	return []string{uuid, "modify", "until:today", "rc.recurrence.confirmation=no"}, nil
}

// StopRecurrence expires a native Taskwarrior recurrence without rewriting
// completed history or deleting generated instances.
func (c *CommandClient) StopRecurrence(ctx context.Context, uuid string) error {
	args, err := StopRecurrenceArgs(uuid)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "stop-recurrence", args...)
	return err
}

// Undo invokes Taskwarrior's native undo command.
func (c *CommandClient) Undo(ctx context.Context) error {
	_, err := c.run(ctx, "undo", "undo")
	return err
}

// ValidateArgs is useful for tests and diagnostics that want to validate an
// argv without executing it.
func ValidateArgs(args []string) error {
	if len(args) == 0 {
		return errors.New("empty Taskwarrior argv")
	}
	for _, arg := range args {
		if strings.ContainsAny(arg, "\x00\r\n") {
			return fmt.Errorf("Taskwarrior argument contains a control character")
		}
	}
	return nil
}
