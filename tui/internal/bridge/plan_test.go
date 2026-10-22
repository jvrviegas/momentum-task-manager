package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestLoadSumsOnlyTheTasksPlannedForToday(t *testing.T) {
	client := &planClient{fakeClient: fakeClient{pending: []domain.Task{
		{UUID: "overdue", Description: "Renew cert", Status: "pending", Due: at(2026, 10, 12, 9), Estimate: estimate(30)},
		{UUID: "due", Description: "Send invoice", Status: "pending", Due: at(2026, 10, 14, 17)},
		{UUID: "tagged", Description: "Review PRs", Status: "pending", Scheduled: at(2026, 10, 14, 9), Estimate: estimate(20), Tags: []string{"momentum-plan-2026-10-14"}},
		{UUID: "rolled", Description: "Write ADR", Status: "pending", Estimate: estimate(45), Tags: []string{"momentum-plan-2026-10-13"}},
		{UUID: "planned", Description: "Refactor ranking", Status: "pending", Estimate: estimate(60), Tags: []string{"momentum-plan-2026-10-14"}},
		{UUID: "unestimated", Description: "Call Ana", Status: "pending", Tags: []string{"momentum-plan-2026-10-14"}},
		{UUID: "idle", Description: "Weekly review", Status: "pending", Estimate: estimate(30)},
	}}}
	settings := config.Defaults()
	settings.Planning.WeekdayCapacity = map[string]string{"wednesday": "6h"}
	load := result[loadResult](t, serve(t, newPlanServer(client, settings), request(t, 1, "load", nil))[1])
	want := planSummary{
		Enabled: true, ChosenCount: 2, ChosenMinutes: 60, UnestimatedChosen: 1,
		FixedMinutes: 50, UnestimatedFixed: 1, CapacityMinutes: 360, BufferMinutes: 60,
	}
	if load.Plan != want {
		t.Fatalf("plan = %+v, want %+v", load.Plan, want)
	}

	settings.Planning.Enabled = false
	load = result[loadResult](t, serve(t, newPlanServer(client, settings), request(t, 1, "load", nil))[1])
	if load.Plan != (planSummary{}) {
		t.Fatalf("plan with planning off = %+v", load.Plan)
	}
}

// calendarFixture is Wednesday 14 October 2026 in local time: a stand-up, a
// review overlapping lunch, a retro running past 17:00, and an early call
// outside working hours.
const calendarFixture = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:standup
DTSTART:20261014T093000
DTEND:20261014T094500
SUMMARY:Team stand-up
END:VEVENT
BEGIN:VEVENT
UID:review
DTSTART:20261014T110000
DTEND:20261014T120000
SUMMARY:Design review
END:VEVENT
BEGIN:VEVENT
UID:lunch
DTSTART:20261014T113000
DTEND:20261014T121500
SUMMARY:Lunch with Rui
END:VEVENT
BEGIN:VEVENT
UID:retro
DTSTART:20261014T163000
DTEND:20261014T173000
SUMMARY:Retro
END:VEVENT
BEGIN:VEVENT
UID:early
DTSTART:20261014T070000
DTEND:20261014T080000
SUMMARY:Early call
END:VEVENT
END:VCALENDAR
`

// calendarSettings reads the fixture, last written at modified, from a
// temporary directory, along with any extra paths.
func calendarSettings(t *testing.T, modified time.Time, extra ...string) config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "work.ics")
	if err := os.WriteFile(path, []byte(calendarFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	settings := config.Defaults()
	settings.Calendar.Enabled = true
	settings.Calendar.Paths = append([]string{path}, extra...)
	return settings
}

func TestPlanCountsMergedCalendarTimeInWorkingHours(t *testing.T) {
	client := &planClient{fakeClient: fakeClient{pending: []domain.Task{
		{UUID: "a", Description: "A", Status: "pending", Estimate: estimate(30)},
	}}}
	server := newPlanServer(client, calendarSettings(t, testNow.Add(-time.Hour)))
	responses := serve(t, server, request(t, 1, "plan", nil), request(t, 2, "load", nil))
	busy := result[planResult](t, responses[1]).Calendar

	block := func(from, to int, titles ...string) busyBlock {
		at := func(hhmm int) string {
			return time.Date(2026, 10, 14, hhmm/100, hhmm%100, 0, 0, time.Local).UTC().Format(time.RFC3339)
		}
		return busyBlock{Start: at(from), End: at(to), Titles: titles}
	}
	want := calendarWire{
		Enabled: true, Available: true, BusyMinutes: 120, EventCount: 5, WorkingStart: "09:00", WorkingEnd: "17:00",
		Blocks: []busyBlock{
			block(930, 945, "Team stand-up"),
			block(1100, 1215, "Design review", "Lunch with Rui"),
			block(1630, 1700, "Retro"),
		},
	}
	if !reflect.DeepEqual(busy, want) {
		t.Fatalf("calendar:\n got %+v\nwant %+v", busy, want)
	}
	summary := result[loadResult](t, responses[2]).Plan
	if summary.CalendarMinutes != 120 || !summary.CalendarAvailable || summary.CalendarError != "" {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestAMissingCalendarFileMakesTheCalendarUnavailable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.ics")
	server := newPlanServer(&planClient{}, calendarSettings(t, testNow, missing))
	responses := serve(t, server, request(t, 1, "plan", nil), request(t, 2, "load", nil))

	plan := result[planResult](t, responses[1])
	busy := plan.Calendar
	if busy.Available || busy.BusyMinutes != 0 || len(busy.Blocks) != 0 || !strings.Contains(busy.Error, "gone.ics") {
		t.Fatalf("calendar = %+v", busy)
	}
	if plan.CapacityMinutes != 480 {
		t.Fatalf("planning should go on with tasks: %+v", plan)
	}
	summary := result[loadResult](t, responses[2]).Plan
	if summary.CalendarMinutes != 0 || summary.CalendarAvailable || summary.CalendarError != busy.Error {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestAnOldCalendarFileIsStaleButCounts(t *testing.T) {
	server := newPlanServer(&planClient{}, calendarSettings(t, testNow.Add(-48*time.Hour)))
	busy := result[planResult](t, serve(t, server, request(t, 1, "plan", nil))[1]).Calendar
	if !busy.Stale || !busy.Available || busy.BusyMinutes != 120 {
		t.Fatalf("calendar = %+v", busy)
	}
}

func TestTheCalendarOffCountsNoTime(t *testing.T) {
	settings := calendarSettings(t, testNow)
	settings.Calendar.Enabled = false
	responses := serve(t, newPlanServer(&planClient{}, settings), request(t, 1, "plan", nil), request(t, 2, "load", nil))
	busy := result[planResult](t, responses[1]).Calendar
	if busy.Enabled || busy.Available || busy.BusyMinutes != 0 || busy.Error != "" || busy.Blocks == nil {
		t.Fatalf("calendar = %+v", busy)
	}
	if summary := result[loadResult](t, responses[2]).Plan; summary.CalendarMinutes != 0 || summary.CalendarAvailable {
		t.Fatalf("summary = %+v", summary)
	}
}
