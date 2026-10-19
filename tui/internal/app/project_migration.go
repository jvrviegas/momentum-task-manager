package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

var (
	ErrProjectMigrationStale    = errors.New("project migration preview is stale")
	ErrProjectMigrationNoClient = errors.New("project migration client is not available")
	ErrProjectMigrationNoStore  = errors.New("project catalog store is not available")
)

// ProjectMigrationClient is the narrow seam used by the coordinator. It reads
// the tasks a migration may change, and applies a confirmed migration in one
// request that changes all of them or, with api.CodeConflict, none (backend
// plan T12). Tests can inject deterministic exports and outcomes.
type ProjectMigrationClient interface {
	ExportPending(context.Context) ([]domain.Task, error)
	ApplyProjectMigration(context.Context, domain.ProjectCatalogPlan, []domain.ProjectTaskMapping) error
}

// ProjectMigrationRequest binds execution to one confirmed catalog plan,
// config snapshot, and exact task UUID set.
type ProjectMigrationRequest struct {
	Plan         domain.ProjectCatalogPlan
	Snapshot     config.ProjectCatalogSnapshot
	TaskMappings []domain.ProjectTaskMapping
	MigrateTasks bool
}

// ProjectMigrationResult separates catalog persistence from the task change.
type ProjectMigrationResult struct {
	CatalogSaved    bool
	CatalogSnapshot config.ProjectCatalogSnapshot
	CatalogErr      error
	PreflightErr    error
	TaskErr         error // why the server changed no task
	Stale           bool
	Canceled        bool
	Changed         int // tasks moved
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
// both stores from being written. If the server refuses the tasks after the
// catalog is saved, the catalog stays saved and the result says why.
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
		if c == nil || c.Client == nil {
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
	err = c.Client.ApplyProjectMigration(ctx, request.Plan, request.TaskMappings)
	var failure *api.Error
	switch {
	case errors.As(err, &failure) && failure.Code == api.CodeConflict:
		result.Stale = true
		result.TaskErr = fmt.Errorf("%w: %s", ErrProjectMigrationStale, failure.Message)
	case err != nil:
		result.TaskErr = err
		result.Canceled = errors.Is(err, context.Canceled)
	default:
		result.Changed = len(request.TaskMappings)
	}
	return result
}

func (c *ProjectMigrationCoordinator) revalidate(ctx context.Context, request ProjectMigrationRequest) error {
	tasks, err := c.Client.ExportPending(ctx)
	if err != nil {
		return fmt.Errorf("preflight project migration: %w", err)
	}
	confirmed, ok := mappingSet(request.TaskMappings)
	if !ok {
		return fmt.Errorf("%w: confirmed task mappings contain duplicate or empty UUIDs", ErrProjectMigrationStale)
	}
	current := request.Plan.PendingTaskMappings(tasks)
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
