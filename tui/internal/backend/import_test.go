package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// export is a Taskwarrior 3.5 export: a task with every field and a
// user-defined one, a weekly series, a waiting task from Taskwarrior 2, and
// an estimate the store cannot hold.
const export = `[
{"id":1,"description":"Send the invoice","due":"20261020T150000Z","entry":"20260703T080000Z","estimate":"PT45M",
 "modified":"20260704T100000Z","priority":"H","project":"work.client","scheduled":"20261019T000000Z","start":"20261015T090000Z",
 "status":"pending","uuid":"1b4e28ba-2fa1-11d2-883f-0016d3cca427","wait":"20261016T000000Z","until":"20261231T000000Z",
 "tags":["next","calls"],"annotations":[{"entry":"20260703T080500Z","description":"Ask for the PO"}],
 "depends":["2c5f39cb-3fb2-12e3-994f-0127e4ddb538"],"client":"acme","urgency":12.5},
{"id":0,"description":"Water the plants","due":"20260901T080000Z","entry":"20260901T080000Z","mask":"X-","modified":"20260901T080000Z",
 "recur":"weekly","rtype":"periodic","status":"recurring","uuid":"4e7f5bed-51d4-1405-b66f-0349f6ffd75a","urgency":0},
{"id":2,"description":"Water the plants","due":"20260908T080000Z","entry":"20260908T080000Z","imask":1,"modified":"20260908T080000Z",
 "parent":"4e7f5bed-51d4-1405-b66f-0349f6ffd75a","recur":"weekly","rtype":"periodic","status":"pending",
 "uuid":"5f8f6cfe-62e5-1516-877f-045af7f0e86b","urgency":2},
{"id":3,"description":"Later","entry":"20260705T080000Z","status":"waiting","wait":"20261101T000000Z",
 "uuid":"2c5f39cb-3fb2-12e3-994f-0127e4ddb538","estimate":"P2Y","urgency":0}
]`

func exported(t *testing.T, data string) []domain.Task {
	t.Helper()
	var tasks []domain.Task
	if err := json.Unmarshal([]byte(data), &tasks); err != nil {
		t.Fatal(err)
	}
	return tasks
}

func when(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return &parsed
}

func TestNewImportConvertsTaskwarriorsExport(t *testing.T) {
	plan, err := NewImport(exported(t, export))
	if err != nil {
		t.Fatal(err)
	}
	first := plan.Tasks[0]
	extra := first.Extra
	first.Extra = nil
	want := api.Task{UUID: "1b4e28ba-2fa1-11d2-883f-0016d3cca427", Description: "Send the invoice", Status: "pending",
		Project: "work.client", Priority: "H", EstimateMinutes: 45, Entry: *when("2026-07-03T08:00:00Z"),
		Modified: *when("2026-07-04T10:00:00Z"), Due: when("2026-10-20T15:00:00Z"), Scheduled: when("2026-10-19T00:00:00Z"),
		Start: when("2026-10-15T09:00:00Z"), Wait: when("2026-10-16T00:00:00Z"), Until: when("2026-12-31T00:00:00Z"),
		Tags: []string{"next", "calls"}, Annotations: []api.Annotation{{Entry: *when("2026-07-03T08:05:00Z"), Description: "Ask for the PO"}},
		Depends: []string{"2c5f39cb-3fb2-12e3-994f-0127e4ddb538"}}
	if !reflect.DeepEqual(first, want) || !reflect.DeepEqual(extra, map[string]json.RawMessage{"client": json.RawMessage(`"acme"`)}) {
		t.Errorf("task:\n got %+v %s\nwant %+v", first, extra, want)
	}
	template, instance, later := plan.Tasks[1], plan.Tasks[2], plan.Tasks[3]
	if template.Status != "recurring" || template.Recurrence != "weekly" || string(template.Extra["mask"]) != `"X-"` ||
		string(template.Extra["rtype"]) != `"periodic"` || len(template.Extra) != 2 {
		t.Errorf("template = %+v", template)
	}
	if instance.Parent != template.UUID || string(instance.Extra["imask"]) != "1" || len(instance.Extra) != 2 {
		t.Errorf("instance = %+v", instance)
	}
	// Taskwarrior 3 keeps a waiting task pending; a missing modified is the
	// entry; an estimate the store cannot hold stays as it was exported.
	if later.Status != "pending" || later.Modified != later.Entry || later.EstimateMinutes != 0 || string(later.Extra["estimate"]) != `"P2Y"` {
		t.Errorf("waiting task = %+v", later)
	}
	if !reflect.DeepEqual(plan.Statuses, map[string]int{"pending": 3, "recurring": 1}) ||
		!reflect.DeepEqual(plan.Extra, map[string]int{"client": 1, "mask": 1, "rtype": 2, "imask": 1, "estimate": 1}) {
		t.Errorf("statuses %v, extra %v", plan.Statuses, plan.Extra)
	}
}

func TestNewImportRefusesRulesMomentumDoesNotSupport(t *testing.T) {
	tasks := exported(t, export)
	tasks[1].Recurrence, tasks[2].Recurrence = "every full moon", "P1W"
	_, err := NewImport(tasks)
	if err == nil || !strings.Contains(err.Error(), `task 4e7f5bed-51d4-1405-b66f-0349f6ffd75a repeats "every full moon"`) ||
		!strings.Contains(err.Error(), `task 5f8f6cfe-62e5-1516-877f-045af7f0e86b repeats "P1W"`) || strings.Contains(err.Error(), "Water the plants") {
		t.Errorf("error = %v", err)
	}
}

func TestImportSendsBatches(t *testing.T) {
	var mu sync.Mutex
	var sizes []string
	mux := http.NewServeMux()
	for _, route := range []string{api.RoutePreviewImport, api.RouteApplyImport} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			var request api.ImportRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &request)
			mu.Lock()
			sizes = append(sizes, fmt.Sprintf("%s %d key=%v %s", r.Pattern, len(request.Tasks), r.Header.Get(api.IdempotencyKeyHeader) != "", r.Header.Get("Authorization")))
			mu.Unlock()
			result := api.ImportResult{Created: []string{request.Tasks[0].UUID}, Unchanged: []string{request.Tasks[1].UUID},
				Different: []api.ImportDifference{{UUID: request.Tasks[2].UUID, Fields: []string{"project"}}}}
			json.NewEncoder(w).Encode(result)
		})
	}
	server := httptest.NewServer(mux)
	defer server.Close()

	tasks := make([]api.Task, 450)
	for i := range tasks {
		tasks[i] = api.Task{UUID: fmt.Sprintf("task-%d", i)}
	}
	plan := Import{Tasks: tasks}
	preview, err := plan.Preview(t.Context(), server.URL, "mtk_device")
	if err != nil || !reflect.DeepEqual(preview.Created, []string{"task-0", "task-200", "task-400"}) ||
		!reflect.DeepEqual(preview.Unchanged, []string{"task-1", "task-201", "task-401"}) || len(preview.Different) != 3 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	if _, err := plan.Apply(t.Context(), server.URL, "mtk_device"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /v1/import/preview 200 key=false Bearer mtk_device", "POST /v1/import/preview 200 key=false Bearer mtk_device",
		"POST /v1/import/preview 50 key=false Bearer mtk_device",
		"POST /v1/import/apply 200 key=true Bearer mtk_device", "POST /v1/import/apply 200 key=true Bearer mtk_device",
		"POST /v1/import/apply 50 key=true Bearer mtk_device",
	}
	if !reflect.DeepEqual(sizes, want) {
		t.Errorf("requests:\n got %q\nwant %q", sizes, want)
	}
	if result, err := (Import{}).Apply(t.Context(), server.URL, "mtk_device"); err != nil || len(result.Created) != 0 || len(sizes) != len(want) {
		t.Errorf("an empty import: %+v, %v, %d requests", result, err, len(sizes))
	}
}
