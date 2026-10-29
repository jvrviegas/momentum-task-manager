package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// roundTrip checks that value encodes to exactly want, the wire format both
// sides rely on, and that want decodes back to value.
func roundTrip[T any](t *testing.T, value T, want string) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %T: %v", value, err)
	}
	if string(data) != want {
		t.Errorf("marshal %T:\n got %s\nwant %s", value, data, want)
	}
	var decoded T
	if err := json.Unmarshal([]byte(want), &decoded); err != nil {
		t.Fatalf("unmarshal %T: %v", value, err)
	}
	if !reflect.DeepEqual(decoded, value) {
		t.Errorf("unmarshal %T:\n got %#v\nwant %#v", value, decoded, value)
	}
}

func at(hour int) time.Time { return time.Date(2026, 10, 14, hour, 30, 0, 0, time.UTC) }

func ptr[T any](value T) *T { return &value }

func TestTaskJSON(t *testing.T) {
	roundTrip(t, Task{
		UUID: "6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10", Description: "Write the plan", Status: StatusPending,
		Project: "work.client", Priority: "H", EstimateMinutes: 90,
		Entry: at(8), Modified: at(9), Due: ptr(at(17)), Scheduled: ptr(at(10)), Start: ptr(at(11)),
		Wait: ptr(at(7)), Until: ptr(at(23)), End: ptr(at(12)),
		Tags:        []string{"next", "writing"},
		Annotations: []Annotation{{Entry: at(9), Description: "Outline agreed"}},
		Depends:     []string{"0d1c6c58-8d4b-4a8e-9f4e-1b2b3c4d5e6f"},
		Recurrence:  "weekly", Parent: "a3f1c2d4-5e6f-4a7b-8c9d-0e1f2a3b4c5d", Urgency: 50.2215,
		Extra: map[string]json.RawMessage{"imask": json.RawMessage(`3`), "rtype": json.RawMessage(`"periodic"`)},
	}, `{"uuid":"6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10","description":"Write the plan","status":"pending",`+
		`"project":"work.client","priority":"H","estimateMinutes":90,`+
		`"entry":"2026-10-14T08:30:00Z","modified":"2026-10-14T09:30:00Z","due":"2026-10-14T17:30:00Z",`+
		`"scheduled":"2026-10-14T10:30:00Z","start":"2026-10-14T11:30:00Z","wait":"2026-10-14T07:30:00Z",`+
		`"until":"2026-10-14T23:30:00Z","end":"2026-10-14T12:30:00Z","tags":["next","writing"],`+
		`"annotations":[{"entry":"2026-10-14T09:30:00Z","description":"Outline agreed"}],`+
		`"depends":["0d1c6c58-8d4b-4a8e-9f4e-1b2b3c4d5e6f"],"recurrence":"weekly",`+
		`"parent":"a3f1c2d4-5e6f-4a7b-8c9d-0e1f2a3b4c5d","urgency":50.2215,"extra":{"imask":3,"rtype":"periodic"}}`)

	// Absent fields are left out, except the ones every task has.
	roundTrip(t, TaskList{Tasks: []Task{{UUID: "u", Description: "d", Status: StatusCompleted, Entry: at(8), Modified: at(9)}}},
		`{"tasks":[{"uuid":"u","description":"d","status":"completed","entry":"2026-10-14T08:30:00Z","modified":"2026-10-14T09:30:00Z","urgency":0}]}`)
}

func TestCreateTaskRequestJSON(t *testing.T) {
	roundTrip(t, CreateTaskRequest{
		Description: "Water plants", Project: "home", Priority: "L", Due: ptr(at(18)), Scheduled: ptr(at(9)),
		Recurrence: "weekly", EstimateMinutes: 15, Tags: []string{"chores"},
	}, `{"description":"Water plants","project":"home","priority":"L","due":"2026-10-14T18:30:00Z",`+
		`"scheduled":"2026-10-14T09:30:00Z","recurrence":"weekly","estimateMinutes":15,"tags":["chores"]}`)
	roundTrip(t, CreateTaskRequest{Description: "Call Ana"}, `{"description":"Call Ana"}`)
}

func TestModifyTaskRequestJSON(t *testing.T) {
	roundTrip(t, ModifyTaskRequest{
		Description:     Change[string]{Kind: domain.Set, Value: "Renamed"},
		Project:         Change[string]{Kind: domain.Clear},
		Due:             Change[time.Time]{Kind: domain.Set, Value: at(17)},
		Scheduled:       Change[time.Time]{Kind: domain.Clear},
		EstimateMinutes: Change[int]{Kind: domain.Set, Value: 45},
		Tags:            &TagChange{Add: []string{"next"}, Remove: []string{"later"}},
	}, `{"description":"Renamed","project":null,"due":"2026-10-14T17:30:00Z","scheduled":null,`+
		`"estimateMinutes":45,"tags":{"add":["next"],"remove":["later"]}}`)
	// Setting a text field to "" is a value, not a clear; the server decides
	// whether it is valid.
	roundTrip(t, ModifyTaskRequest{Priority: Change[string]{Kind: domain.Set}, Recurrence: Change[string]{Kind: domain.Clear}},
		`{"priority":"","recurrence":null}`)
	roundTrip(t, ModifyTaskRequest{}, `{}`)
}

func TestProjectJSON(t *testing.T) {
	roundTrip(t, ProjectList{Projects: []string{"home", "work.client"}}, `{"projects":["home","work.client"]}`)
	roundTrip(t, TagList{Tags: []string{"next"}}, `{"tags":["next"]}`)
	migration := ProjectMigration{Source: "work", Destination: "job", IncludeSubprojects: true}
	roundTrip(t, migration, `{"source":"work","destination":"job","includeSubprojects":true}`)
	tasks := []ProjectTaskMapping{{UUID: "u1", OldValue: "work", NewValue: "job"}, {UUID: "u2", OldValue: "work.client", NewValue: "job.client"}}
	tasksJSON := `[{"uuid":"u1","oldValue":"work","newValue":"job"},{"uuid":"u2","oldValue":"work.client","newValue":"job.client"}]`
	roundTrip(t, ProjectMigrationPreview{Tasks: tasks}, `{"tasks":`+tasksJSON+`}`)
	roundTrip(t, ApplyProjectMigrationRequest{ProjectMigration: migration, Tasks: tasks},
		`{"source":"work","destination":"job","includeSubprojects":true,"tasks":`+tasksJSON+`}`)
	roundTrip(t, ProjectMigrationResult{Changed: 2}, `{"changed":2}`)
}

func TestImportJSON(t *testing.T) {
	roundTrip(t, ImportRequest{Tasks: []Task{{UUID: "6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10", Description: "Old", Status: StatusDeleted,
		Entry: at(9), Modified: at(10), End: ptr(at(10)), Extra: map[string]json.RawMessage{"imask": json.RawMessage(`1`)}}}},
		`{"tasks":[{"uuid":"6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10","description":"Old","status":"deleted",`+
			`"entry":"2026-10-14T09:30:00Z","modified":"2026-10-14T10:30:00Z","end":"2026-10-14T10:30:00Z","urgency":0,"extra":{"imask":1}}]}`)
	roundTrip(t, ImportResult{Created: []string{"a"}, Unchanged: []string{}, Different: []ImportDifference{{UUID: "b", Fields: []string{"project", "due"}}}},
		`{"created":["a"],"unchanged":[],"different":[{"uuid":"b","fields":["project","due"]}]}`)
}

func TestVersionJSON(t *testing.T) {
	roundTrip(t, Version{Server: "v0.1.0"}, `{"server":"v0.1.0"}`)
	roundTrip(t, TimeZone{TimeZone: "Europe/Lisbon"}, `{"timeZone":"Europe/Lisbon"}`)
}

func TestErrorJSON(t *testing.T) {
	roundTrip(t, Error{Code: CodeInvalid, Message: "priority must be H, M, L, or empty", Field: "priority"},
		`{"code":"invalid","message":"priority must be H, M, L, or empty","field":"priority"}`)
	roundTrip(t, Error{Code: CodeNotFound, Message: "task not found"}, `{"code":"not_found","message":"task not found"}`)
	var err error = &Error{Code: CodeConflict, Message: "the task changed since; undo is not available"}
	if err.Error() != "the task changed since; undo is not available" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestAuthJSON(t *testing.T) {
	roundTrip(t, DeviceSignIn{
		DeviceCode: "device-code", UserCode: "BCDF-GHJK", VerificationURL: "https://momentum.example/device",
		VerificationURLComplete: "https://momentum.example/device?code=BCDF-GHJK", ExpiresAt: at(9), IntervalSeconds: 5,
	}, `{"deviceCode":"device-code","userCode":"BCDF-GHJK","verificationUrl":"https://momentum.example/device",`+
		`"verificationUrlComplete":"https://momentum.example/device?code=BCDF-GHJK","expiresAt":"2026-10-14T09:30:00Z","intervalSeconds":5}`)
	roundTrip(t, PollDeviceSignInRequest{DeviceCode: "device-code"}, `{"deviceCode":"device-code"}`)
	roundTrip(t, Token{Token: "mtk_secret", ExpiresAt: at(9), Email: "alice@example.com"},
		`{"token":"mtk_secret","expiresAt":"2026-10-14T09:30:00Z","email":"alice@example.com"}`)
	roundTrip(t, Session{Email: "alice@example.com", ExpiresAt: at(9)}, `{"email":"alice@example.com","expiresAt":"2026-10-14T09:30:00Z"}`)
	roundTrip(t, Error{Code: CodeAuthorizationPending, Message: "finish signing in in the browser"},
		`{"code":"authorization_pending","message":"finish signing in in the browser"}`)
}

func TestGoogleCalendarJSON(t *testing.T) {
	roundTrip(t, GoogleCalendarLink{URL: "https://momentum.example/calendar/connect/abc", ExpiresAt: at(9)},
		`{"url":"https://momentum.example/calendar/connect/abc","expiresAt":"2026-10-14T09:30:00Z"}`)
	roundTrip(t, GoogleCalendarStatus{Offered: true, Connected: true, Email: "alice@gmail.com"},
		`{"offered":true,"connected":true,"email":"alice@gmail.com","needsReconnect":false}`)
	roundTrip(t, GoogleCalendarStatus{Offered: true, Connected: true, Email: "alice@gmail.com", NeedsReconnect: true},
		`{"offered":true,"connected":true,"email":"alice@gmail.com","needsReconnect":true}`)
	// A server that does not offer Google Calendar sets nothing else.
	roundTrip(t, GoogleCalendarStatus{}, `{"offered":false,"connected":false,"needsReconnect":false}`)
	roundTrip(t, GoogleCalendarDisconnect{Revoked: false}, `{"revoked":false}`)
	roundTrip(t, GoogleCalendarEvents{Events: []GoogleCalendarEvent{
		{Title: "Standup", Start: at(9), End: at(10)},
		{Title: "Offsite", Start: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), AllDay: true, Transparent: true},
	}}, `{"events":[{"title":"Standup","start":"2026-10-14T09:30:00Z","end":"2026-10-14T10:30:00Z","allDay":false,"transparent":false},`+
		`{"title":"Offsite","start":"2026-10-14T00:00:00Z","end":"2026-10-15T00:00:00Z","allDay":true,"transparent":true}]}`)
	roundTrip(t, GoogleCalendarEvents{Events: []GoogleCalendarEvent{}}, `{"events":[]}`)
	roundTrip(t, Error{Code: CodeCalendarUnavailable, Message: "Google Calendar needs reconnecting"},
		`{"code":"calendar_unavailable","message":"Google Calendar needs reconnecting"}`)
}

func TestRoutesAreDistinctV1Patterns(t *testing.T) {
	mux := http.NewServeMux()
	for _, route := range []string{
		RouteVersion, RouteListTasks, RouteCreateTask, RouteModifyTask, RouteDeleteTask, RouteCompleteTask,
		RouteStartTask, RouteStopTask, RouteStopRecurrence, RouteUndo, RouteListProjects, RouteListTags,
		RoutePreviewProjectMigration, RouteApplyProjectMigration, RouteSetTimeZone,
		RouteStartDeviceSignIn, RoutePollDeviceSignIn, RouteSession, RouteSignOut,
		RoutePreviewImport, RouteApplyImport,
		RouteConnectGoogleCalendar, RouteGoogleCalendarStatus, RouteDisconnectGoogleCalendar, RouteGoogleCalendarEvents,
	} {
		method, path, _ := strings.Cut(route, " ")
		if method == "" || !strings.HasPrefix(path, "/v1/") {
			t.Errorf("route %q is not a method and a /v1/ path", route)
		}
		if strings.Contains(path, "{") && !strings.Contains(path, "{"+PathUUID+"}") {
			t.Errorf("route %q has a wildcard other than {%s}", route, PathUUID)
		}
		// Handle panics on an invalid pattern or one that conflicts with another.
		mux.HandleFunc(route, func(http.ResponseWriter, *http.Request) {})
	}
}
