// Package backend keeps the apps' tasks on a Momentum server (backend plan
// T13 and T19): the TUI and the bridge reach it through Client.
package backend

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// Client reaches one Momentum server as one signed-in device. It resolves
// date expressions in the device's time zone before sending them (D3).
type Client struct {
	server *api.Client
	zone   string
	now    func() time.Time

	mu       sync.Mutex
	zoneSent bool
}

// New returns a client for the server at url that signs its requests with
// token. Before its first change, it makes zone, the device's IANA time zone,
// the user's, so their series follow it; an empty zone leaves the user's as
// it is.
func New(url, token, zone string) *Client {
	return &Client{server: api.NewClient(url, token), zone: zone, now: time.Now}
}

// ExportPending returns the pending tasks that are not waiting.
func (c *Client) ExportPending(ctx context.Context) ([]domain.Task, error) {
	return c.list(ctx, api.StatusPending, time.Time{})
}

// ExportCompleted returns the tasks completed after the instant after.
func (c *Client) ExportCompleted(ctx context.Context, after time.Time) ([]domain.Task, error) {
	return c.list(ctx, api.StatusCompleted, after)
}

func (c *Client) list(ctx context.Context, status string, endedAfter time.Time) ([]domain.Task, error) {
	found, err := c.server.ListTasks(ctx, status, endedAfter)
	if err != nil {
		return nil, err
	}
	tasks := make([]domain.Task, len(found))
	for i, task := range found {
		tasks[i] = toTask(task)
	}
	return tasks, nil
}

// Projects returns the projects of pending tasks that are not waiting.
func (c *Client) Projects(ctx context.Context) ([]string, error) { return c.server.Projects(ctx) }

// Version answers which server this is, which a diagnostics check shows.
func (c *Client) Version(ctx context.Context) (api.Version, error) { return c.server.Version(ctx) }

// Session answers who this device is signed in as, and until when.
func (c *Client) Session(ctx context.Context) (api.Session, error) { return c.server.Session(ctx) }

// Tags returns the tags of pending tasks that are not waiting.
func (c *Client) Tags(ctx context.Context) ([]string, error) { return c.server.Tags(ctx) }

func (c *Client) Add(ctx context.Context, input domain.NewTask) error {
	request, err := createRequest(input, c.now())
	if err != nil {
		return err
	}
	return c.change(ctx, func() (api.Task, error) { return c.server.CreateTask(ctx, request) })
}

func (c *Client) Modify(ctx context.Context, uuid string, diff domain.TaskDiff) error {
	request, err := modifyRequest(diff, c.now())
	if err != nil {
		return err
	}
	return c.change(ctx, func() (api.Task, error) { return c.server.ModifyTask(ctx, uuid, request) })
}

// AddNote adds a note with text to task uuid; the server gives it its time.
func (c *Client) AddNote(ctx context.Context, uuid, text string) error {
	return c.modifyTask(ctx, uuid, api.ModifyTaskRequest{Annotations: &api.AnnotationChange{Add: []string{text}}})
}

// RemoveNote removes the note of task uuid that was added at entry with text.
func (c *Client) RemoveNote(ctx context.Context, uuid string, entry time.Time, text string) error {
	remove := []api.Annotation{{Entry: entry, Description: text}}
	return c.modifyTask(ctx, uuid, api.ModifyTaskRequest{Annotations: &api.AnnotationChange{Remove: remove}})
}

// AddDependency makes task uuid depend on task on.
func (c *Client) AddDependency(ctx context.Context, uuid, on string) error {
	return c.modifyTask(ctx, uuid, api.ModifyTaskRequest{Depends: &api.DependencyChange{Add: []string{on}}})
}

// RemoveDependency stops task uuid depending on task on.
func (c *Client) RemoveDependency(ctx context.Context, uuid, on string) error {
	return c.modifyTask(ctx, uuid, api.ModifyTaskRequest{Depends: &api.DependencyChange{Remove: []string{on}}})
}

func (c *Client) modifyTask(ctx context.Context, uuid string, request api.ModifyTaskRequest) error {
	return c.change(ctx, func() (api.Task, error) { return c.server.ModifyTask(ctx, uuid, request) })
}

// Duplicate adds a task with the description, project, priority, dates,
// estimate and tags of the pending task uuid, and returns its UUID (plan D4).
// The daily plan's tag stays with the original, and the copy does not repeat.
func (c *Client) Duplicate(ctx context.Context, uuid string) (string, error) {
	pending, err := c.server.ListTasks(ctx, api.StatusPending, time.Time{})
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(pending, func(task api.Task) bool { return task.UUID == uuid })
	if i < 0 {
		return "", &api.Error{Code: api.CodeConflict, Message: "the task is no longer pending"}
	}
	original := pending[i]
	request := api.CreateTaskRequest{
		Description: original.Description, Project: original.Project, Priority: original.Priority,
		Due: original.Due, Scheduled: original.Scheduled, EstimateMinutes: original.EstimateMinutes,
		Tags: slices.DeleteFunc(slices.Clone(original.Tags), domain.IsPlanTag),
	}
	var created api.Task
	err = c.change(ctx, func() (api.Task, error) {
		created, err = c.server.CreateTask(ctx, request)
		return created, err
	})
	return created.UUID, err
}

func (c *Client) Complete(ctx context.Context, uuid string) error {
	return c.change(ctx, func() (api.Task, error) { return c.server.CompleteTask(ctx, uuid) })
}

func (c *Client) Delete(ctx context.Context, uuid string) error {
	return c.change(ctx, func() (api.Task, error) { return c.server.DeleteTask(ctx, uuid) })
}

func (c *Client) Start(ctx context.Context, uuid string) error {
	return c.change(ctx, func() (api.Task, error) { return c.server.StartTask(ctx, uuid) })
}

func (c *Client) Stop(ctx context.Context, uuid string) error {
	return c.change(ctx, func() (api.Task, error) { return c.server.StopTask(ctx, uuid) })
}

// StopRecurrence ends the series of uuid, a template or an instance, at the
// start of today in the user's time zone.
func (c *Client) StopRecurrence(ctx context.Context, uuid string) error {
	return c.change(ctx, func() (api.Task, error) { return c.server.StopRecurrence(ctx, uuid) })
}

// Undo reverts this device's latest change, if its tasks have not changed
// since.
func (c *Client) Undo(ctx context.Context) error {
	return c.change(ctx, func() (api.Task, error) { return c.server.Undo(ctx) })
}

// ApplyProjectMigration moves plan's tasks as tasks map them, in one request:
// the server changes all of them or, with api.CodeConflict when they no
// longer match, none.
func (c *Client) ApplyProjectMigration(ctx context.Context, plan domain.ProjectCatalogPlan, tasks []domain.ProjectTaskMapping) error {
	request := api.ApplyProjectMigrationRequest{
		ProjectMigration: api.ProjectMigration{Source: plan.SourceValue, Destination: plan.DestinationValue, IncludeSubprojects: plan.IncludeSubprojects},
		Tasks:            make([]api.ProjectTaskMapping, len(tasks)),
	}
	for i, task := range tasks {
		request.Tasks[i] = api.ProjectTaskMapping(task)
	}
	return c.change(ctx, func() (api.Task, error) {
		_, err := c.server.ApplyProjectMigration(ctx, request)
		return api.Task{}, err
	})
}

// change sets the user's time zone if this client has not yet, then sends.
func (c *Client) change(ctx context.Context, send func() (api.Task, error)) error {
	if err := c.sendZone(ctx); err != nil {
		return err
	}
	_, err := send()
	return err
}

func (c *Client) sendZone(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.zoneSent || c.zone == "" {
		return nil
	}
	if _, err := c.server.SetTimeZone(ctx, c.zone); err != nil {
		return err
	}
	c.zoneSent = true
	return nil
}
