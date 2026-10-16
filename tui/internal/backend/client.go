// Package backend keeps the apps' tasks on a Momentum server instead of in
// Taskwarrior (backend plan T13). Client implements the Taskwarrior adapter's
// interfaces, so the TUI and the bridge reach either store through the same
// calls.
package backend

import (
	"context"
	"sync"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/taskwarrior"
)

// Client reaches one Momentum server as one signed-in device. It resolves
// date expressions in the device's time zone before sending them (D3), and
// answers the Taskwarrior-only calls itself: backend mode has no sync (P1),
// no contexts (D6), and always stores estimates.
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

// Version returns the server's version.
func (c *Client) Version(ctx context.Context) (string, error) {
	version, err := c.server.Version(ctx)
	return version.Server, err
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

// Sync has nothing to do: a change is on the server once it answers.
func (c *Client) Sync(context.Context) (taskwarrior.SyncResult, error) {
	return taskwarrior.SyncResult{}, nil
}

// SyncConfigured is false, so the apps skip their sync schedule and the
// unsynced-changes prompt.
func (c *Client) SyncConfigured(context.Context) (bool, error) { return false, nil }

// Context is empty: backend mode has no Taskwarrior contexts.
func (c *Client) Context(context.Context) (string, error) { return "", nil }

// EstimateUDAReadiness is always configured: the server stores estimates.
func (c *Client) EstimateUDAReadiness(context.Context) (taskwarrior.EstimateUDAState, error) {
	return taskwarrior.EstimateUDAConfigured, nil
}
