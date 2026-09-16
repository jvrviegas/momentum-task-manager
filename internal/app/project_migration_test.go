package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jvrviegas/momentum/internal/config"
	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
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

type coordinatorClient struct {
	export    taskwarrior.ProjectMigrationExport
	exportErr error
	outcomes  []taskwarrior.ProjectTaskOutcome
	calls     []domain.ProjectTaskMapping
	events    *[]string
	cancel    context.CancelFunc
}

func (c *coordinatorClient) ExportPendingInContext(context.Context) (taskwarrior.ProjectMigrationExport, error) {
	if c.exportErr != nil {
		return taskwarrior.ProjectMigrationExport{}, c.exportErr
	}
	return c.export, nil
}

func (c *coordinatorClient) ApplyProjectTask(_ context.Context, _ taskwarrior.ProjectMigrationScope, mapping domain.ProjectTaskMapping) taskwarrior.ProjectTaskOutcome {
	if c.events != nil {
		*c.events = append(*c.events, "task:"+mapping.UUID)
	}
	c.calls = append(c.calls, mapping)
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if len(c.outcomes) == 0 {
		return taskwarrior.ProjectTaskOutcome{Mapping: mapping, Kind: taskwarrior.ProjectTaskChanged}
	}
	outcome := c.outcomes[0]
	c.outcomes = c.outcomes[1:]
	outcome.Mapping = mapping
	return outcome
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

func coordinatorRequest(plan domain.ProjectCatalogPlan, scope taskwarrior.ProjectMigrationScope, mappings []domain.ProjectTaskMapping, migrate bool) ProjectMigrationRequest {
	return ProjectMigrationRequest{Plan: plan, Scope: scope, TaskMappings: mappings, MigrateTasks: migrate}
}

func TestProjectMigrationCoordinatorSavesCatalogBeforeConfirmedTasks(t *testing.T) {
	events := []string{}
	plan := coordinatorPlan(t, false)
	scope := taskwarrior.ProjectMigrationScope{}
	mapping := domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"}
	store := &coordinatorStore{events: &events}
	client := &coordinatorClient{
		export:   taskwarrior.ProjectMigrationExport{Scope: scope, Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}},
		outcomes: []taskwarrior.ProjectTaskOutcome{{Kind: taskwarrior.ProjectTaskChanged}},
		events:   &events,
	}
	result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, scope, []domain.ProjectTaskMapping{mapping}, true))
	if result.Stale || result.CatalogErr != nil || !result.CatalogSaved || len(result.TaskOutcomes) != 1 || result.TaskOutcomes[0].Kind != taskwarrior.ProjectTaskChanged {
		t.Fatalf("result=%#v", result)
	}
	if !reflect.DeepEqual(events, []string{"catalog-save", "task:one"}) || len(store.saved) != 1 || len(client.calls) != 1 {
		t.Fatalf("ordering/events=%v saved=%v calls=%v", events, store.saved, client.calls)
	}
}

func TestProjectMigrationCoordinatorCatalogFailureRunsNoTaskCommands(t *testing.T) {
	plan := coordinatorPlan(t, false)
	mapping := domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"}
	storeErr := errors.New("config write failed")
	store := &coordinatorStore{err: storeErr}
	client := &coordinatorClient{export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}}}
	result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, taskwarrior.ProjectMigrationScope{}, []domain.ProjectTaskMapping{mapping}, true))
	if result.CatalogSaved || !errors.Is(result.CatalogErr, storeErr) || len(client.calls) != 0 || len(result.TaskOutcomes) != 0 {
		t.Fatalf("result=%#v calls=%v", result, client.calls)
	}
}

func TestProjectMigrationCoordinatorRejectsStaleScopeOrExactSetBeforeSave(t *testing.T) {
	plan := coordinatorPlan(t, false)
	mapping := domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"}
	cases := []struct {
		name   string
		scope  taskwarrior.ProjectMigrationScope
		export taskwarrior.ProjectMigrationExport
	}{
		{
			name:  "changed context",
			scope: taskwarrior.ProjectMigrationScope{ContextName: "work", ReadFilter: "project:work"},
			export: taskwarrior.ProjectMigrationExport{
				Scope: taskwarrior.ProjectMigrationScope{ContextName: "personal", ReadFilter: "project:personal"},
				Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}},
			},
		},
		{
			name:   "new task entered scope",
			scope:  taskwarrior.ProjectMigrationScope{},
			export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}, {UUID: "two", Status: "pending", Project: "work"}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &coordinatorStore{}
			client := &coordinatorClient{export: tc.export}
			result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, tc.scope, []domain.ProjectTaskMapping{mapping}, true))
			if !result.Stale || result.CatalogSaved || len(store.saved) != 0 || len(client.calls) != 0 {
				t.Fatalf("result=%#v saved=%v calls=%v", result, store.saved, client.calls)
			}
		})
	}
}

func TestProjectMigrationCoordinatorStopsAfterPartialFailureAndRetainsCatalog(t *testing.T) {
	plan := coordinatorPlan(t, false)
	mappings := []domain.ProjectTaskMapping{
		{UUID: "one", OldValue: "work", NewValue: "delivery"},
		{UUID: "two", OldValue: "work", NewValue: "delivery"},
		{UUID: "three", OldValue: "work", NewValue: "delivery"},
	}
	outcomes := []taskwarrior.ProjectTaskOutcome{
		{Kind: taskwarrior.ProjectTaskChanged},
		{Kind: taskwarrior.ProjectTaskFailed, Err: errors.New("hook failed")},
	}
	store := &coordinatorStore{}
	client := &coordinatorClient{
		export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{
			{UUID: "one", Status: "pending", Project: "work"},
			{UUID: "two", Status: "pending", Project: "work"},
			{UUID: "three", Status: "pending", Project: "work"},
		}},
		outcomes: outcomes,
	}
	result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, taskwarrior.ProjectMigrationScope{}, mappings, true))
	if !result.CatalogSaved || result.CatalogErr != nil || len(result.TaskOutcomes) != 2 || len(client.calls) != 2 || result.TaskOutcomes[1].Kind != taskwarrior.ProjectTaskFailed {
		t.Fatalf("result=%#v calls=%v", result, client.calls)
	}
	if len(store.saved) != 1 || store.saved[0][0].Value != "delivery" {
		t.Fatalf("catalog was not retained: %#v", store.saved)
	}
}

func TestProjectMigrationCoordinatorStopsAtFirstMiddleOrLastFailure(t *testing.T) {
	plan := coordinatorPlan(t, false)
	mappings := []domain.ProjectTaskMapping{
		{UUID: "one", OldValue: "work", NewValue: "delivery"},
		{UUID: "two", OldValue: "work", NewValue: "delivery"},
		{UUID: "three", OldValue: "work", NewValue: "delivery"},
	}
	for _, tc := range []struct {
		name          string
		failureAt     int
		wantCallCount int
	}{
		{name: "first", failureAt: 0, wantCallCount: 1},
		{name: "middle", failureAt: 1, wantCallCount: 2},
		{name: "last", failureAt: 2, wantCallCount: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcomes := make([]taskwarrior.ProjectTaskOutcome, len(mappings))
			for i := range outcomes {
				outcomes[i].Kind = taskwarrior.ProjectTaskChanged
			}
			outcomes[tc.failureAt] = taskwarrior.ProjectTaskOutcome{Kind: taskwarrior.ProjectTaskFailed, Err: errors.New("failure")}
			client := &coordinatorClient{
				export:   taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}, {UUID: "two", Status: "pending", Project: "work"}, {UUID: "three", Status: "pending", Project: "work"}}},
				outcomes: outcomes,
			}
			result := NewProjectMigrationCoordinator(&coordinatorStore{}, client).Execute(context.Background(), coordinatorRequest(plan, taskwarrior.ProjectMigrationScope{}, mappings, true))
			if !result.CatalogSaved || len(client.calls) != tc.wantCallCount || len(result.TaskOutcomes) != tc.wantCallCount || result.TaskOutcomes[tc.failureAt].Kind != taskwarrior.ProjectTaskFailed {
				t.Fatalf("result=%#v calls=%v", result, client.calls)
			}
		})
	}
}

func TestProjectMigrationCoordinatorReportsPreflightFailureWithoutSaving(t *testing.T) {
	plan := coordinatorPlan(t, false)
	store := &coordinatorStore{}
	clientErr := errors.New("Taskwarrior unavailable")
	client := &coordinatorClient{exportErr: clientErr}
	result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, taskwarrior.ProjectMigrationScope{}, nil, true))
	if result.CatalogSaved || result.Stale || !errors.Is(result.PreflightErr, clientErr) || len(store.saved) != 0 {
		t.Fatalf("result=%#v saved=%v", result, store.saved)
	}
}

func TestProjectMigrationCoordinatorHandlesZeroMatchesWithoutTaskWrites(t *testing.T) {
	plan := coordinatorPlan(t, false)
	store := &coordinatorStore{}
	client := &coordinatorClient{export: taskwarrior.ProjectMigrationExport{Scope: taskwarrior.ProjectMigrationScope{}}}
	result := NewProjectMigrationCoordinator(store, client).Execute(context.Background(), coordinatorRequest(plan, taskwarrior.ProjectMigrationScope{}, nil, true))
	if result.Stale || !result.CatalogSaved || result.CatalogErr != nil || len(client.calls) != 0 || result.ChangedCount() != 0 {
		t.Fatalf("result=%#v calls=%v", result, client.calls)
	}
}

func TestProjectMigrationCoordinatorSupportsCatalogOnlyWithoutMigrationClient(t *testing.T) {
	plan := coordinatorPlan(t, false)
	store := &coordinatorStore{}
	result := NewProjectMigrationCoordinator(store, nil).Execute(context.Background(), coordinatorRequest(plan, taskwarrior.ProjectMigrationScope{}, nil, false))
	if !result.CatalogSaved || result.CatalogErr != nil || len(store.saved) != 1 {
		t.Fatalf("result=%#v saved=%v", result, store.saved)
	}
}

func TestProjectMigrationCoordinatorReportsAmbiguousOutcomeAndDoesNotRetry(t *testing.T) {
	plan := coordinatorPlan(t, false)
	mapping := domain.ProjectTaskMapping{UUID: "one", OldValue: "work", NewValue: "delivery"}
	client := &coordinatorClient{
		export:   taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}},
		outcomes: []taskwarrior.ProjectTaskOutcome{{Kind: taskwarrior.ProjectTaskAmbiguous, Err: errors.New("timeout")}},
	}
	result := NewProjectMigrationCoordinator(&coordinatorStore{}, client).Execute(context.Background(), coordinatorRequest(plan, taskwarrior.ProjectMigrationScope{}, []domain.ProjectTaskMapping{mapping}, true))
	if !result.CatalogSaved || len(result.TaskOutcomes) != 1 || result.TaskOutcomes[0].Kind != taskwarrior.ProjectTaskAmbiguous || len(client.calls) != 1 {
		t.Fatalf("result=%#v calls=%v", result, client.calls)
	}
}

func TestProjectMigrationCoordinatorStopsAfterCancellationAndRetainsPriorOutcomes(t *testing.T) {
	plan := coordinatorPlan(t, false)
	mappings := []domain.ProjectTaskMapping{
		{UUID: "one", OldValue: "work", NewValue: "delivery"},
		{UUID: "two", OldValue: "work", NewValue: "delivery"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	client := &coordinatorClient{
		export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{
			{UUID: "one", Status: "pending", Project: "work"},
			{UUID: "two", Status: "pending", Project: "work"},
		}},
		cancel: cancel,
	}
	result := NewProjectMigrationCoordinator(&coordinatorStore{}, client).Execute(ctx, coordinatorRequest(plan, taskwarrior.ProjectMigrationScope{}, mappings, true))
	if !result.CatalogSaved || !result.Canceled || len(result.TaskOutcomes) != 1 || len(client.calls) != 1 {
		t.Fatalf("result=%#v calls=%v", result, client.calls)
	}
}

func TestProjectMigrationCoordinatorHonorsCanceledContextBeforeWrites(t *testing.T) {
	plan := coordinatorPlan(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &coordinatorStore{}
	client := &coordinatorClient{export: taskwarrior.ProjectMigrationExport{Tasks: []domain.Task{{UUID: "one", Status: "pending", Project: "work"}}}}
	result := NewProjectMigrationCoordinator(store, client).Execute(ctx, coordinatorRequest(plan, taskwarrior.ProjectMigrationScope{}, nil, false))
	if !errors.Is(result.CatalogErr, context.Canceled) || result.CatalogSaved || len(store.saved) != 0 {
		t.Fatalf("result=%#v saved=%v", result, store.saved)
	}
}
