package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/calendar"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// The macOS app decodes the bridge's answers with the types in
// macos/Momentum/Bridge/Wire.swift. Each answer below is kept as a file in
// testdata/wire, and MomentumTests/WireContractTests.swift decodes every file,
// so renaming a JSON key fails a test on the side that did not change instead
// of the running app. After changing the wire on purpose, run
//
//	go test ./internal/bridge -run TestWire -update
//
// and commit the files that changed.
var updateWire = flag.Bool("update", false, "rewrite the files in testdata/wire")

// wireNow is in UTC so the files do not depend on the machine's time zone.
var wireNow = time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)

// wireUTC makes the machine's time zone UTC for one test. A calendar file's
// modified time is the one answer that is written in it.
func wireUTC(t *testing.T) {
	t.Helper()
	local := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = local })
}

func wireTime(day, hour, minute int) *time.Time {
	value := time.Date(2026, 10, day, hour, minute, 0, 0, time.UTC)
	return &value
}

func wirePtr[T any](value T) *T { return &value }

// wireServerFor is a bridge on a config.toml of its own in a temporary folder,
// which it returns so the files can hide it.
func wireServerFor(t *testing.T, client Client, text string) (*Server, string) {
	t.Helper()
	file := newConfigFile(t, text)
	settings, _, err := config.LoadWithOptions(config.LoadOptions{PathOverride: file.path})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(func(string) Client { return client }, settings, file.path)
	server.now = func() time.Time { return wireNow }
	return server, filepath.Dir(file.path)
}

// wireAnswer sends one request and checks its answer, or its refusal, against
// testdata/wire/<name>.json. Each string in hide, such as a temporary folder,
// is written as <dir>.
func wireAnswer(t *testing.T, server *Server, name, op string, params any, hide ...string) {
	t.Helper()
	response := serve(t, server, request(t, 1, op, params))[1]
	body := []byte(response.Result)
	if response.Error != nil {
		var err error
		if body, err = json.Marshal(response.Error); err != nil {
			t.Fatal(err)
		}
	}
	var shown bytes.Buffer
	if err := json.Indent(&shown, body, "", "  "); err != nil {
		t.Fatalf("%s: %v in %s", name, err, body)
	}
	text := shown.String() + "\n"
	for _, value := range hide {
		text = strings.ReplaceAll(text, value, "<dir>")
	}
	path := filepath.Join("testdata", "wire", name+".json")
	if *updateWire {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v; run the test with -update to write it", name, err)
	}
	if string(want) != text {
		t.Errorf("%s no longer matches %s; if the wire changed on purpose, run the test with -update.\n got: %swant: %s", name, path, text, want)
	}
}

// wireTask has every field the app reads, so that a renamed key shows.
func wireTask() domain.Task {
	return domain.Task{
		UUID: "6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10", ID: 7, Description: "Send invoice", Status: "pending",
		Project: "work.client", Priority: "H", Estimate: estimate(90),
		Entry: wireTime(10, 9, 0), Modified: wireTime(13, 9, 0), Due: wireTime(14, 16, 0), Scheduled: wireTime(14, 9, 0),
		Start: wireTime(14, 9, 30), Wait: wireTime(13, 8, 0), Until: wireTime(30, 0, 0),
		Tags:         []string{"finance", "momentum-plan-2026-10-14"},
		Annotations:  []domain.Annotation{{Description: "Ask for the PO number", Entry: wireTime(13, 11, 0)}},
		Dependencies: []string{"0d3c8f7f-3c39-4a51-9d55-6b9a5f9e1a10"},
		Recurrence:   "weekly", Parent: "3f0f2a54-51a9-4d55-8a2b-9b1e2c0d7e11", Urgency: 12.4,
		RawFields: map[string]json.RawMessage{"customer": json.RawMessage(`"Acme"`)},
	}
}

const wireProjects = backendTable + `
[[projects]]
name = "Work"
value = "work"

[[projects]]
name = "Client"
value = "work.client"
`

func TestWireTasksAndCapture(t *testing.T) {
	wireUTC(t)
	client := &fakeClient{
		pending: []domain.Task{
			wireTask(),
			{UUID: "overdue", ID: 8, Description: "Renew cert", Status: "pending", Due: wireTime(12, 9, 0), Estimate: estimate(30), Urgency: 9},
			{UUID: "inbox", ID: 9, Description: "Weekly review", Status: "pending", Project: "work"},
		},
		completed: []domain.Task{
			{UUID: "done", ID: 10, Description: "Pay rent", Status: "completed", Project: "home", End: wireTime(13, 18, 0)},
		},
		projects: []string{"work", "work.client", "home"},
	}
	server, dir := wireServerFor(t, client, wireProjects)

	wireAnswer(t, server, "status", "status", nil, dir)
	wireAnswer(t, server, "load", "load", nil)
	wireAnswer(t, server, "interpret", "interpret", interpretParams{Input: "Call Ana tomorrow #work"})
	wireAnswer(t, server, "suggest", "suggest", suggestParams{Input: "Fix #wo now", Cursor: 7})
	wireAnswer(t, server, "capture-added", "capture", interpretParams{Input: "Call Ana tomorrow"})
	wireAnswer(t, server, "capture-review", "capture", interpretParams{Input: "Call at 3pm"})
	wireAnswer(t, server, "modify", "modify", modifyParams{
		UUID:   wireTask().UUID,
		Before: editFields{Description: "Send invoice"},
		After:  editFields{Description: "Send the invoice"},
	})
	wireAnswer(t, server, "duplicate", "duplicate", uuidParams{UUID: wireTask().UUID})
}

func TestWirePlanAndGoogleCalendar(t *testing.T) {
	wireUTC(t)
	pending := func() []domain.Task {
		return []domain.Task{
			{UUID: "overdue", Description: "Renew cert", Status: "pending", Due: wireTime(12, 9, 0), Estimate: estimate(30), Urgency: 9},
			{UUID: "due", Description: "Send invoice", Status: "pending", Due: wireTime(14, 17, 0)},
			{UUID: "rolled", Description: "Write ADR", Status: "pending", Tags: []string{"docs", "momentum-plan-2026-10-12"}},
			{UUID: "planned", Description: "Refactor ranking", Status: "pending", Tags: []string{"momentum-plan-2026-10-14"}, Urgency: 3},
			{UUID: "idle", Description: "Weekly review", Status: "pending", Due: wireTime(16, 9, 0)},
		}
	}
	event := func(summary string, from, to *time.Time) calendar.Event {
		return calendar.Event{Summary: summary, Start: *from, End: *to, Source: "google"}
	}
	google := func() *googleClient {
		return &googleClient{
			planClient: planClient{fakeClient: fakeClient{pending: pending()}},
			events: []calendar.Event{
				event("Team stand-up", wireTime(14, 9, 30), wireTime(14, 9, 45)),
				event("Design review", wireTime(14, 11, 0), wireTime(14, 12, 0)),
				event("Retro", wireTime(14, 16, 30), wireTime(14, 17, 30)),
			},
			status: api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "joao@gmail.com"},
			link:   api.GoogleCalendarLink{URL: "https://accounts.example/o/oauth2/auth?state=abc", ExpiresAt: *wireTime(14, 10, 10)},
			left:   api.GoogleCalendarDisconnect{Revoked: false},
		}
	}
	const googleSource = backendTable + "[calendar]\nsource = \"google\"\n"

	server, _ := wireServerFor(t, google(), googleSource)
	wireAnswer(t, server, "plan", "plan", nil)
	wireAnswer(t, server, "calendar-status", "calendarStatus", nil)
	wireAnswer(t, server, "calendar-connect", "connectCalendar", nil)
	wireAnswer(t, server, "calendar-disconnect", "disconnectCalendar", nil)

	notOffered := google()
	notOffered.status = api.GoogleCalendarStatus{}
	server, _ = wireServerFor(t, notOffered, googleSource)
	wireAnswer(t, server, "calendar-status-not-offered", "calendarStatus", nil)

	reconnect := google()
	reconnect.status = api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "joao@gmail.com", NeedsReconnect: true}
	server, _ = wireServerFor(t, reconnect, googleSource)
	wireAnswer(t, server, "calendar-status-needs-reconnect", "calendarStatus", nil)

	saved := &planClient{fakeClient: fakeClient{pending: pending()}}
	server, _ = wireServerFor(t, saved, backendTable)
	wireAnswer(t, server, "apply-plan", "applyPlan", applyPlanParams{Date: "2026-10-14", UUIDs: []string{"planned", "due"}})

	stopped := &planClient{fakeClient: fakeClient{pending: pending()}, failAt: 2}
	server, _ = wireServerFor(t, stopped, backendTable)
	wireAnswer(t, server, "apply-plan-stopped", "applyPlan", applyPlanParams{Date: "2026-10-14", UUIDs: []string{"idle", "due"}})
}

func TestWireSettingsProjectsAndDoctor(t *testing.T) {
	wireUTC(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	folder := filepath.Join(home, "Calendars")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(folder, "fresh.ics")
	if err := os.WriteFile(fresh, []byte("BEGIN:VEVENT\nDTSTART:20261014T100000Z\nDTEND:20261014T110000Z\nEND:VEVENT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fresh, wireNow.Add(-time.Hour), wireNow.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	server, dir := wireServerFor(t, &fakeClient{}, backendTable+`[planning]
daily_capacity = "7h30m"
buffer = "45m"
[planning.weekday_capacity]
friday = "4h"
[calendar]
enabled = true
paths = ["~/Calendars/fresh.ics", "~/Calendars/gone.ics"]
`)
	wireAnswer(t, server, "settings", "settings", nil, dir, home)

	// A refused save names the field, which the app shows by it.
	revision := settingsOf(t, server).Revision
	wireAnswer(t, server, "error-field", "savePlanning", savePlanningParams{Revision: revision, DailyCapacity: wirePtr("a day")})

	server, dir = wireServerFor(t, &fakeClient{}, planning6h)
	server.Version, server.DeviceZone = "v1.2.3", "Europe/Lisbon"
	wireAnswer(t, server, "doctor", "doctor", nil, dir)

	projects := &migrationClient{fakeClient: fakeClient{pending: []domain.Task{
		{UUID: "1", Description: "Task 1", Status: "pending", Project: "work"},
		{UUID: "2", Description: "Task 2", Status: "pending", Project: "work.client"},
		{UUID: "3", Description: "Task 3", Status: "pending", Project: "side.gig"},
	}}}
	server, dir = wireServerFor(t, projects, projectsConfig)
	wireAnswer(t, server, "projects", "projects", nil, dir)

	revision = projectsOf(t, server).Revision
	rename := projectEditParams{Revision: revision, Action: "edit", Source: "work", Name: "Job", Segment: "job", IncludeSubprojects: true}
	wireAnswer(t, server, "project-preview", "previewMigration", rename, dir)
	rename.Migrate, rename.Tasks = true, previewOf(t, server, rename).SubprojectTasks
	wireAnswer(t, server, "project-save", "saveProject", rename, dir)
}

// completedFails cannot list completed tasks.
type completedFails struct{ fakeClient }

func (*completedFails) ExportCompleted(context.Context, time.Time) ([]domain.Task, error) {
	return nil, errors.New("the server could not list completed tasks")
}

// The server leaves out its keys of trouble when there is none, so each needs
// an answer of its own, or a renamed key would pass unseen.
func TestWireTroubleKeys(t *testing.T) {
	wireUTC(t)
	file := newConfigFile(t, planning6h)
	server := file.server()
	file.write(backendTable + "[calendar]\nenabled = true\n")
	wireAnswer(t, server, "status-config-error", "status", nil, filepath.Dir(file.path))

	file = newConfigFile(t, planning6h)
	server = file.server()
	file.write("[backend]\nurl = \"https://other.example\"\n")
	wireAnswer(t, server, "status-restart-required", "status", nil, filepath.Dir(file.path))

	home := t.TempDir()
	t.Setenv("HOME", home)
	client := &completedFails{fakeClient{pending: []domain.Task{{
		UUID: "warned", ID: 11, Description: "Odd estimate", Status: "pending",
		EstimateWarning: "estimate must be a whole number of minutes",
		RawFields:       map[string]json.RawMessage{"estimate": json.RawMessage(`"soon"`)},
	}}}}
	server, dir := wireServerFor(t, client, backendTable+"[calendar]\nenabled = true\npaths = [\"~/Calendars/gone.ics\"]\n")
	wireAnswer(t, server, "load-trouble", "load", nil, dir, home)

	broken := &migrationClient{exportErr: errors.New("the server could not list tasks")}
	server, dir = wireServerFor(t, broken, projectsConfig)
	wireAnswer(t, server, "projects-tasks-error", "projects", nil, dir)

	stale := &migrationClient{
		fakeClient: fakeClient{pending: []domain.Task{{UUID: "1", Description: "Task 1", Status: "pending", Project: "work"}}},
		applyErr:   &api.Error{Code: api.CodeConflict, Message: "tasks changed"},
	}
	server, dir = wireServerFor(t, stale, projectsConfig)
	edit := projectEditParams{Revision: projectsOf(t, server).Revision, Action: "edit", Source: "work", Name: "Job", Segment: "job"}
	edit.Migrate, edit.Tasks = true, previewOf(t, server, edit).ExactTasks
	wireAnswer(t, server, "project-save-stale", "saveProject", edit, dir)
}

func TestWireSignInAndErrors(t *testing.T) {
	wireUTC(t)
	server, _ := newSigningInBridge(t)
	wireAnswer(t, server, "sign-in-start", "signInStart", nil)

	server, _ = newSigningInBridge(t, pollError(api.CodeAuthorizationPending))
	wireAnswer(t, server, "sign-in-pending", "signInPoll", signInPollParams{DeviceCode: "dc"})

	server, _ = newSigningInBridge(t, `{"token":"mtk_new","expiresAt":"2027-01-14T12:00:00+00:00","email":"ana@example.com"}`)
	wireAnswer(t, server, "sign-in-done", "signInPoll", signInPollParams{DeviceCode: "dc"})

	refusing := newBackendServer(t, map[string]func(string) (int, string){
		api.RouteSetTimeZone: answer(401, `{"code":"unauthorized","message":"sign in again"}`),
		api.RouteListTasks:   answer(401, `{"code":"unauthorized","message":"sign in again"}`),
	})
	wireAnswer(t, newBackendBridge(refusing.url), "error-unauthorized", "load", nil)
}
