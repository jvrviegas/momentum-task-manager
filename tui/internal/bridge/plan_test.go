package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// planClient applies each Modify's tag changes to its pending tasks, as the
// server does, and refuses the change numbered failAt.
type planClient struct {
	fakeClient
	modifies int
	failAt   int
}

func (c *planClient) Modify(_ context.Context, uuid string, diff domain.TaskDiff) error {
	c.modifies++
	if c.modifies == c.failAt {
		return errors.New("the server refused the change")
	}
	for i, task := range c.pending {
		if task.UUID != uuid {
			continue
		}
		tags := slices.DeleteFunc(slices.Clone(task.Tags), func(tag string) bool { return slices.Contains(diff.Tags.Remove, tag) })
		c.pending[i].Tags = append(tags, diff.Tags.Add...)
	}
	c.modified = append(c.modified, diff)
	return nil
}

func (c *planClient) tags(uuid string) []string {
	for _, task := range c.pending {
		if task.UUID == uuid {
			return task.Tags
		}
	}
	return nil
}

func newPlanServer(client Client, settings config.Config) *Server {
	settings.Backend.URL = "https://momentum.example"
	server := NewServer(func(string) Client { return client }, settings, "/tmp/config.toml")
	server.now = func() time.Time { return testNow }
	return server
}

func result[T any](t *testing.T, response rawResponse) T {
	t.Helper()
	var value T
	if response.Error != nil {
		t.Fatalf("error: %+v", response.Error)
	}
	if err := json.Unmarshal(response.Result, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func estimate(minutes int) *domain.Estimate { return &domain.Estimate{Minutes: minutes} }

func TestPlanGroupsTasksAndPreselectsRollover(t *testing.T) {
	client := &planClient{fakeClient: fakeClient{pending: []domain.Task{
		{UUID: "overdue", Description: "Renew cert", Status: "pending", Due: at(2026, 10, 12, 9), Estimate: estimate(30), Urgency: 9},
		{UUID: "due", Description: "Send invoice", Status: "pending", Due: at(2026, 10, 14, 17)},
		{UUID: "rolled", Description: "Write ADR", Status: "pending", Tags: []string{"docs", "momentum-plan-2026-10-12"}},
		{UUID: "planned", Description: "Refactor ranking", Status: "pending", Tags: []string{"momentum-plan-2026-10-14"}, Urgency: 3},
		{UUID: "idle", Description: "Weekly review", Status: "pending", Due: at(2026, 10, 16, 9)},
	}}}
	settings := config.Defaults()
	settings.Planning.WeekdayCapacity = map[string]string{"wednesday": "6h"}
	responses := serve(t, newPlanServer(client, settings), request(t, 1, "plan", nil))
	plan := result[planResult](t, responses[1])

	if plan.Date != "2026-10-14" || plan.Tag != "momentum-plan-2026-10-14" || plan.CapacityMinutes != 360 || plan.BufferMinutes != 60 {
		t.Fatalf("plan = %+v", plan)
	}
	describe := func(items []planItemWire) string {
		values := []string{}
		for _, item := range items {
			value := item.Task.UUID
			if item.Selected {
				value += "+"
			}
			if item.PlannedDate != "" {
				value += "@" + item.PlannedDate
			}
			values = append(values, value)
		}
		return strings.Join(values, ",")
	}
	for name, got := range map[string]string{
		"overdue,due":        describe(plan.Obligations),
		"rolled+@2026-10-12": describe(plan.Rollover),
		"planned+,idle":      describe(plan.Candidates),
	} {
		if got != name {
			t.Errorf("got %q, want %q", got, name)
		}
	}
	if plan.Obligations[0].Task.EstimateMinutes != 30 || plan.Obligations[0].Task.TodayGroup != "Overdue" {
		t.Errorf("obligation task = %+v", plan.Obligations[0].Task)
	}
}

func TestApplyPlanAnswersTotalZeroWhenUnchanged(t *testing.T) {
	client := &planClient{fakeClient: fakeClient{pending: []domain.Task{
		{UUID: "planned", Description: "Refactor ranking", Status: "pending", Tags: []string{"momentum-plan-2026-10-14"}},
		{UUID: "idle", Description: "Weekly review", Status: "pending"},
	}}}
	responses := serve(t, newPlanServer(client, config.Defaults()),
		request(t, 1, "applyPlan", applyPlanParams{Date: "2026-10-14", UUIDs: []string{"planned"}}))
	if got := result[applyPlanResult](t, responses[1]); got != (applyPlanResult{}) || client.modifies != 0 {
		t.Fatalf("result = %+v, modifies = %d", got, client.modifies)
	}
}

func TestApplyPlanStopsAtAFailureAndARetryFinishes(t *testing.T) {
	client := &planClient{failAt: 2, fakeClient: fakeClient{pending: []domain.Task{
		{UUID: "a", Description: "A", Status: "pending"},
		{UUID: "b", Description: "B", Status: "pending"},
		{UUID: "c", Description: "C", Status: "pending"},
	}}}
	server := newPlanServer(client, config.Defaults())
	params := applyPlanParams{Date: "2026-10-14", UUIDs: []string{"a", "b", "c"}}

	first := result[applyPlanResult](t, serve(t, server, request(t, 1, "applyPlan", params))[1])
	if first != (applyPlanResult{Applied: 1, Total: 3, Error: "the server refused the change"}) {
		t.Fatalf("first = %+v", first)
	}
	retry := result[applyPlanResult](t, serve(t, server, request(t, 2, "applyPlan", params))[2])
	if retry != (applyPlanResult{Applied: 2, Total: 2}) {
		t.Fatalf("retry = %+v", retry)
	}
	for _, uuid := range []string{"a", "b", "c"} {
		if got := client.tags(uuid); !slices.Equal(got, []string{"momentum-plan-2026-10-14"}) {
			t.Errorf("%s tags = %v", uuid, got)
		}
	}
}

func TestApplyPlanSkipsATaskCompletedSinceThePlanOpened(t *testing.T) {
	client := &planClient{fakeClient: fakeClient{pending: []domain.Task{
		{UUID: "a", Description: "A", Status: "pending"},
		{UUID: "b", Description: "B", Status: "pending"},
	}}}
	server := newPlanServer(client, config.Defaults())
	plan := result[planResult](t, serve(t, server, request(t, 1, "plan", nil))[1])
	client.pending = client.pending[:1] // b was completed elsewhere

	got := result[applyPlanResult](t, serve(t, server,
		request(t, 2, "applyPlan", applyPlanParams{Date: plan.Date, UUIDs: []string{"a", "b"}}))[2])
	if got != (applyPlanResult{Applied: 1, Total: 1}) || len(client.modified) != 1 {
		t.Fatalf("result = %+v, modified = %v", got, client.modified)
	}
}

func TestApplyPlanWritesTheGivenDate(t *testing.T) {
	client := &planClient{fakeClient: fakeClient{pending: []domain.Task{
		{UUID: "a", Description: "A", Status: "pending", Tags: []string{"home", "momentum-plan-2026-10-12"}},
	}}}
	// The sheet opened on the 13th and was confirmed after midnight.
	responses := serve(t, newPlanServer(client, config.Defaults()),
		request(t, 1, "applyPlan", applyPlanParams{Date: "2026-10-13", UUIDs: []string{"a"}}),
		request(t, 2, "applyPlan", applyPlanParams{Date: "13/10/2026", UUIDs: []string{"a"}}),
	)
	if got := result[applyPlanResult](t, responses[1]); got != (applyPlanResult{Applied: 1, Total: 1}) {
		t.Fatalf("result = %+v", got)
	}
	if got := client.tags("a"); !slices.Equal(got, []string{"home", "momentum-plan-2026-10-13"}) {
		t.Errorf("tags = %v", got)
	}
	if got := responses[2].Error; got == nil || got.Message != `invalid plan date "13/10/2026"` {
		t.Errorf("invalid date: %+v", got)
	}
}
