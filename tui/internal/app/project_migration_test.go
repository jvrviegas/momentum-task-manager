package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

type coordinatorStore struct {
	err      error
	saved    []domain.ProjectCatalog
	events   *[]string
	snapshot config.ProjectCatalogSnapshot
}

func (s *coordinatorStore) Read(context.Context) (config.ProjectCatalogSnapshot, error) {
	return s.snapshot, nil
}

func (s *coordinatorStore) Save(_ context.Context, _ config.ProjectCatalogSnapshot, projects domain.ProjectCatalog) (config.ProjectCatalogSnapshot, error) {
	if s.events != nil {
		*s.events = append(*s.events, "catalog-save")
	}
	if s.err != nil {
		return config.ProjectCatalogSnapshot{}, s.err
	}
	s.saved = append(s.saved, append(domain.ProjectCatalog(nil), projects...))
	s.snapshot.Projects = append(domain.ProjectCatalog(nil), projects...)
	return s.snapshot, nil
}

// coordinatorClient previews a migration from tasks and applies it in one
// request, as the server does.
type coordinatorClient struct {
	tasks     []domain.Task
	exportErr error
	err       error
	plans     []domain.ProjectCatalogPlan
	calls     [][]domain.ProjectTaskMapping
	events    *[]string
}

func (c *coordinatorClient) ExportPending(context.Context) ([]domain.Task, error) {
	return c.tasks, c.exportErr
}

func (c *coordinatorClient) ApplyProjectMigration(_ context.Context, plan domain.ProjectCatalogPlan, mappings []domain.ProjectTaskMapping) error {
	if c.events != nil {
		*c.events = append(*c.events, "apply")
	}
	c.plans = append(c.plans, plan)
	c.calls = append(c.calls, mappings)
	return c.err
}

func coordinatorPlan(t *testing.T, includeSubprojects bool) domain.ProjectCatalogPlan {
	t.Helper()
	catalog := domain.ProjectCatalog{{Name: "Work", Value: "work"}}
	plan, err := domain.PlanUpdateProject(catalog, "work", domain.Project{Name: "Delivery", Value: "delivery"}, includeSubprojects)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func coordinatorRequest(plan domain.ProjectCatalogPlan, mappings []domain.ProjectTaskMapping, migrate bool) ProjectMigrationRequest {
	return ProjectMigrationRequest{Plan: plan, TaskMappings: mappings, MigrateTasks: migrate}
}

func TestProjectMigrationCoordinatorAppliesTasksAfterTheCatalog(t *testing.T) {
	plan := coordinatorPlan(t, true)
	mappings := []domain.ProjectTaskMapping{{UUID: "one", OldValue: "work", NewValue: "delivery"}, {UUID: "two", OldValue: "work.client", NewValue: "delivery.client"}}
	tasks := []domain.Task{{UUID: "one", Status: "pending", Project: "work"}, {UUID: "two", Status: "pending", Project: "work.client"}}

	events := []string{}
	store := &coordinatorStore{events: &events}
	client := &coordinatorClient{tasks: tasks, events: &events}
	result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, mappings, true))
	if !result.CatalogSaved || result.Stale || result.TaskErr != nil || result.Changed != 2 {
		t.Fatalf("result=%#v", result)
	}
	if !reflect.DeepEqual(events, []string{"catalog-save", "apply"}) || !reflect.DeepEqual(client.calls, [][]domain.ProjectTaskMapping{mappings}) || !client.plans[0].IncludeSubprojects {
		t.Fatalf("events=%v calls=%v plans=%v", events, client.calls, client.plans)
	}
}

func TestProjectMigrationCoordinatorKeepsTheCatalogWhenTheServerRefuses(t *testing.T) {
	plan := coordinatorPlan(t, false)
	mappings := []domain.ProjectTaskMapping{{UUID: "one", OldValue: "work", NewValue: "delivery"}}
	tasks := []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}
	for _, test := range []struct {
		name  string
		err   error
		stale bool
	}{
		{name: "tasks changed", err: &api.Error{Code: api.CodeConflict, Message: "the tasks changed since the preview"}, stale: true},
		{name: "unreachable", err: api.ErrUnreachable},
	} {
		store := &coordinatorStore{}
		client := &coordinatorClient{tasks: tasks, err: test.err}
		result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, mappings, true))
		if !result.CatalogSaved || len(store.saved) != 1 || result.Stale != test.stale || result.Changed != 0 || result.TaskErr == nil ||
			errors.Is(result.TaskErr, ErrProjectMigrationStale) != test.stale {
			t.Errorf("%s: result=%#v", test.name, result)
		}
	}
}

func TestProjectMigrationCoordinatorCatalogFailureAppliesNoTasks(t *testing.T) {
	plan := coordinatorPlan(t, false)
	mapping := domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"}
	storeErr := errors.New("config write failed")
	store := &coordinatorStore{err: storeErr}
	client := &coordinatorClient{tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}}
	result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, []domain.ProjectTaskMapping{mapping}, true))
	if result.CatalogSaved || !errors.Is(result.CatalogErr, storeErr) || len(client.calls) != 0 || result.Changed != 0 {
		t.Fatalf("result=%#v calls=%v", result, client.calls)
	}
}

func TestProjectMigrationCoordinatorRejectsAStalePreviewBeforeSave(t *testing.T) {
	plan := coordinatorPlan(t, false)
	mapping := domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"}
	store := &coordinatorStore{}
	client := &coordinatorClient{tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}, {UUID: "two", Status: "pending", Project: "work"}}}
	result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, []domain.ProjectTaskMapping{mapping}, true))
	if !result.Stale || result.CatalogSaved || len(store.saved) != 0 || len(client.calls) != 0 {
		t.Fatalf("result=%#v saved=%v calls=%v", result, store.saved, client.calls)
	}
}

func TestProjectMigrationCoordinatorReportsPreflightFailureWithoutSaving(t *testing.T) {
	plan := coordinatorPlan(t, false)
	store := &coordinatorStore{}
	clientErr := errors.New("server unavailable")
	client := &coordinatorClient{exportErr: clientErr}
	result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, nil, true))
	if result.CatalogSaved || result.Stale || !errors.Is(result.PreflightErr, clientErr) || len(store.saved) != 0 {
		t.Fatalf("result=%#v saved=%v", result, store.saved)
	}
}

func TestProjectMigrationCoordinatorHandlesZeroMatchesWithoutTaskWrites(t *testing.T) {
	plan := coordinatorPlan(t, false)
	store := &coordinatorStore{}
	client := &coordinatorClient{}
	result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, nil, true))
	if result.Stale || !result.CatalogSaved || result.CatalogErr != nil || len(client.calls) != 0 || result.Changed != 0 {
		t.Fatalf("result=%#v calls=%v", result, client.calls)
	}
}

func TestProjectMigrationCoordinatorSupportsCatalogOnlyWithoutMigrationClient(t *testing.T) {
	plan := coordinatorPlan(t, false)
	store := &coordinatorStore{}
	result := NewProjectMigrationCoordinator(store, nil).Execute(context.Background(), coordinatorRequest(plan, nil, false))
	if !result.CatalogSaved || result.CatalogErr != nil || len(store.saved) != 1 {
		t.Fatalf("result=%#v saved=%v", result, store.saved)
	}
}

func TestProjectMigrationCoordinatorNeedsAClientToMoveTasks(t *testing.T) {
	store := &coordinatorStore{}
	mapping := domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"}
	result := NewProjectMigrationCoordinator(store, nil).Execute(context.Background(), coordinatorRequest(coordinatorPlan(t, false), []domain.ProjectTaskMapping{mapping}, true))
	if !errors.Is(result.PreflightErr, ErrProjectMigrationNoClient) || result.CatalogSaved || len(store.saved) != 0 {
		t.Errorf("result=%#v", result)
	}
}

func TestProjectMigrationCoordinatorHonorsCanceledContextBeforeWrites(t *testing.T) {
	plan := coordinatorPlan(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &coordinatorStore{}
	client := &coordinatorClient{tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}}
	result := NewProjectMigrationCoordinator(store, client).Execute(ctx, coordinatorRequest(plan, nil, false))
	if !errors.Is(result.CatalogErr, context.Canceled) || result.CatalogSaved || len(store.saved) != 0 {
		t.Fatalf("result=%#v saved=%v", result, store.saved)
	}
}
