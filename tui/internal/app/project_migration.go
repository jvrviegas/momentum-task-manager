package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/taskwarrior"
)

var (
	ErrProjectMigrationStale    = errors.New("project migration preview is stale")
	ErrProjectMigrationNoClient = errors.New("project migration client is not available")
	ErrProjectMigrationNoStore  = errors.New("project catalog store is not available")
)

// ProjectMigrationClient is the narrow seam used by the coordinator. It reads
// the tasks a migration may change, and applies a confirmed migration task by
// task (ProjectTaskApplier, as Taskwarrior's CommandClient does) or in one
// request (ProjectMigrationApplier, as backend mode does). Tests can inject
// deterministic exports and outcomes.
type ProjectMigrationClient interface {
	ExportPendingInContext(context.Context) (taskwarrior.ProjectMigrationExport, error)
}

// ProjectTaskApplier applies one confirmed mapping with a guarded command.
type ProjectTaskApplier interface {
	ApplyProjectTask(context.Context, taskwarrior.ProjectMigrationScope, domain.ProjectTaskMapping) taskwarrior.ProjectTaskOutcome
}

// ProjectMigrationApplier applies every confirmed mapping in one request that
// changes all of them or, with api.CodeConflict, none (backend plan T12).
type ProjectMigrationApplier interface {
	ApplyProjectMigration(context.Context, domain.ProjectCatalogPlan, []domain.ProjectTaskMapping) error
}

// ProjectMigrationRequest binds execution to one confirmed catalog plan,
// config snapshot, active context, and exact task UUID set.
type ProjectMigrationRequest struct {
	Plan         domain.ProjectCatalogPlan
	Snapshot     config.ProjectCatalogSnapshot
	Scope        taskwarrior.ProjectMigrationScope
	TaskMappings []domain.ProjectTaskMapping
	MigrateTasks bool
}

// ProjectMigrationResult separates catalog persistence from every task outcome.
type ProjectMigrationResult struct {
	CatalogSaved    bool
	CatalogSnapshot config.ProjectCatalogSnapshot
	CatalogErr      error
	PreflightErr    error
	TaskErr         error // why a migration applied in one request changed no task
	Stale           bool
	Canceled        bool
	TaskOutcomes    []taskwarrior.ProjectTaskOutcome
}

func (r ProjectMigrationResult) ChangedCount() int {
	return r.count(taskwarrior.ProjectTaskChanged)
}

func (r ProjectMigrationResult) SkippedCount() int {
	return r.count(taskwarrior.ProjectTaskSkipped)
}

func (r ProjectMigrationResult) FailedCount() int {
	return r.count(taskwarrior.ProjectTaskFailed)
}

func (r ProjectMigrationResult) AmbiguousCount() int {
	return r.count(taskwarrior.ProjectTaskAmbiguous)
}

func (r ProjectMigrationResult) count(kind taskwarrior.ProjectTaskOutcomeKind) int {
	count := 0
	for _, outcome := range r.TaskOutcomes {
		if outcome.Kind == kind {
			count++
		}
	}
	return count
}

// ProjectMigrationCoordinator performs the deliberately non-transactional
// catalog-first sequence: fresh preflight, catalog save, then only the
// confirmed UUID mappings. It never retries or expands a plan.
type ProjectMigrationCoordinator struct {
	Store  config.ProjectCatalogStore
	Client ProjectMigrationClient
}

func NewProjectMigrationCoordinator(store config.ProjectCatalogStore, client ProjectMigrationClient) *ProjectMigrationCoordinator {
	return &ProjectMigrationCoordinator{Store: store, Client: client}
}

// Execute runs one confirmed project migration. A stale preflight prevents
// both stores from being written. Once the catalog is saved, successful prior
// task outcomes remain visible if a later task fails or becomes ambiguous.
func (c *ProjectMigrationCoordinator) Execute(ctx context.Context, request ProjectMigrationRequest) ProjectMigrationResult {
	result := ProjectMigrationResult{}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		result.CatalogErr = err
		result.Canceled = errors.Is(err, context.Canceled)
		return result
	}
	if request.MigrateTasks {
		if !c.canApply() {
			result.PreflightErr = ErrProjectMigrationNoClient
			return result
		}
		if err := c.revalidate(ctx, request); err != nil {
			result.PreflightErr = err
			result.Stale = errors.Is(err, ErrProjectMigrationStale)
			return result
		}
	}
	if c == nil || c.Store == nil {
		result.CatalogErr = ErrProjectMigrationNoStore
		return result
	}
	snapshot, err := c.Store.Save(ctx, request.Snapshot, request.Plan.After)
	if err != nil {
		result.CatalogErr = err
		result.Canceled = errors.Is(err, context.Canceled)
		return result
	}
	result.CatalogSaved = true
	result.CatalogSnapshot = snapshot
	if !request.MigrateTasks || len(request.TaskMappings) == 0 {
		return result
	}
	if applier, ok := c.Client.(ProjectMigrationApplier); ok {
		return c.applyAll(ctx, applier, request, result)
	}

	applier := c.Client.(ProjectTaskApplier)
	for _, mapping := range request.TaskMappings {
		if err := ctx.Err(); err != nil {
			result.Canceled = errors.Is(err, context.Canceled)
			result.PreflightErr = err
			break
		}
		outcome := applier.ApplyProjectTask(ctx, request.Scope, mapping)
		result.TaskOutcomes = append(result.TaskOutcomes, outcome)
		if outcome.Kind == taskwarrior.ProjectTaskFailed || outcome.Kind == taskwarrior.ProjectTaskAmbiguous {
			break
		}
	}
	return result
}

func (c *ProjectMigrationCoordinator) canApply() bool {
	if c == nil || c.Client == nil {
		return false
	}
	_, batch := c.Client.(ProjectMigrationApplier)
	_, perTask := c.Client.(ProjectTaskApplier)
	return batch || perTask
}

// applyAll applies the confirmed mappings in one request after the catalog is
// saved. A refusal because the tasks changed since the preview is stale.
func (c *ProjectMigrationCoordinator) applyAll(ctx context.Context, applier ProjectMigrationApplier, request ProjectMigrationRequest, result ProjectMigrationResult) ProjectMigrationResult {
	err := applier.ApplyProjectMigration(ctx, request.Plan, request.TaskMappings)
	var failure *api.Error
	switch {
	case errors.As(err, &failure) && failure.Code == api.CodeConflict:
		result.Stale = true
		result.TaskErr = fmt.Errorf("%w: %s", ErrProjectMigrationStale, failure.Message)
	case err != nil:
		result.TaskErr = err
		result.Canceled = errors.Is(err, context.Canceled)
	default:
		for _, mapping := range request.TaskMappings {
			result.TaskOutcomes = append(result.TaskOutcomes, taskwarrior.ProjectTaskOutcome{Mapping: mapping, Kind: taskwarrior.ProjectTaskChanged})
		}
	}
	return result
}

func (c *ProjectMigrationCoordinator) revalidate(ctx context.Context, request ProjectMigrationRequest) error {
	export, err := c.Client.ExportPendingInContext(ctx)
	if err != nil {
		return fmt.Errorf("preflight project migration: %w", err)
	}
	if export.Scope != request.Scope {
		return fmt.Errorf("%w: active context changed from %q to %q", ErrProjectMigrationStale, request.Scope.ContextName, export.Scope.ContextName)
	}
	confirmed, ok := mappingSet(request.TaskMappings)
	if !ok {
		return fmt.Errorf("%w: confirmed task mappings contain duplicate or empty UUIDs", ErrProjectMigrationStale)
	}
	current := request.Plan.PendingTaskMappings(export.Tasks)
	fresh, ok := mappingSet(current)
	if !ok || !sameMappingSet(confirmed, fresh) {
		return fmt.Errorf("%w: eligible UUID/value mappings changed", ErrProjectMigrationStale)
	}
	return nil
}

func mappingSet(mappings []domain.ProjectTaskMapping) (map[string]domain.ProjectTaskMapping, bool) {
	result := make(map[string]domain.ProjectTaskMapping, len(mappings))
	for _, mapping := range mappings {
		if mapping.UUID == "" {
			return nil, false
		}
		if _, exists := result[mapping.UUID]; exists {
			return nil, false
		}
		result[mapping.UUID] = mapping
	}
	return result, true
}

func sameMappingSet(left, right map[string]domain.ProjectTaskMapping) bool {
	if len(left) != len(right) {
		return false
	}
	for uuid, mapping := range left {
		if mapping != right[uuid] {
			return false
		}
	}
	return true
}
