package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

type fakeClient struct {
	pending   []domain.Task
	completed []domain.Task
	projects  []string
	added     []domain.NewTask
	modified  []domain.TaskDiff
	actions   []string
	undoGate  chan struct{}
}

func (f *fakeClient) ExportPending(context.Context) ([]domain.Task, error) {
	return f.pending, nil
}
func (f *fakeClient) ExportCompleted(context.Context, time.Time) ([]domain.Task, error) {
	return f.completed, nil
}
func (f *fakeClient) Projects(context.Context) ([]string, error) { return f.projects, nil }
func (f *fakeClient) Add(_ context.Context, task domain.NewTask) error {
	f.added = append(f.added, task)
	return nil
}
func (f *fakeClient) Modify(_ context.Context, uuid string, diff domain.TaskDiff) error {
	f.modified = append(f.modified, diff)
	return nil
}
func (f *fakeClient) Complete(_ context.Context, uuid string) error { return f.act("complete " + uuid) }
func (f *fakeClient) Delete(_ context.Context, uuid string) error   { return f.act("delete " + uuid) }
func (f *fakeClient) Start(_ context.Context, uuid string) error    { return f.act("start " + uuid) }
func (f *fakeClient) Stop(_ context.Context, uuid string) error     { return f.act("stop " + uuid) }
func (f *fakeClient) StopRecurrence(_ context.Context, uuid string) error {
	return f.act("stop-recurrence " + uuid)
}
func (f *fakeClient) Undo(context.Context) error {
	if f.undoGate != nil {
		<-f.undoGate
	}
	return f.act("undo")
}
func (f *fakeClient) act(name string) error {
	f.actions = append(f.actions, name)
	return nil
}

var testNow = time.Date(2026, 10, 14, 10, 0, 0, 0, time.Local)

// newTestServer serves client as the [backend] server, which settings name
// unless a test sets its own.
func newTestServer(client *fakeClient, settings config.Config) *Server {
	if !settings.Backend.Enabled() {
		settings.Backend.URL = "https://momentum.example"
	}
	server := NewServer(func(string) Client { return client }, settings, "/tmp/config.toml")
	server.now = func() time.Time { return testNow }
	return server
}

type rawResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *Error          `json:"error"`
}

// serve sends each request line and returns responses keyed by request ID.
func serve(t *testing.T, server *Server, lines ...string) map[int64]rawResponse {
	t.Helper()
	var output strings.Builder
	if err := server.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")), &output); err != nil {
		t.Fatal(err)
	}
	responses := map[int64]rawResponse{}
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var response rawResponse
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		responses[response.ID] = response
	}
	return responses
}

func request(t *testing.T, id int64, op string, params any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"id": id, "op": op, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func at(year int, month time.Month, day, hour int) *time.Time {
	value := time.Date(year, month, day, hour, 0, 0, 0, time.Local)
	return &value
}

func TestPureOperationsAnswerWhileTheServerIsBusy(t *testing.T) {
	client := &fakeClient{undoGate: make(chan struct{})}
	server := newTestServer(client, config.Defaults())
	input, writer := io.Pipe()
	output, sink := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background(), input, sink); sink.Close() }()
	lines := make(chan string)
	go func() {
		responses := bufio.NewScanner(output)
		for responses.Scan() {
			lines <- responses.Text()
		}
		close(lines)
	}()
	next := func() string {
		select {
		case line := <-lines:
			return line
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for a response")
			return ""
		}
	}

	go func() {
		_, _ = io.WriteString(writer, request(t, 1, "undo", nil)+"\n")
		_, _ = io.WriteString(writer, request(t, 2, "interpret", map[string]any{"input": "Call mom tomorrow"})+"\n")
	}()
	if line := next(); !strings.Contains(line, `"id":2`) {
		t.Fatalf("interpret should answer before the blocked undo, got %q", line)
	}
	close(client.undoGate)
	if line := next(); !strings.Contains(line, `"id":1`) {
		t.Fatalf("undo response missing, got %q", line)
	}
	writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestServerOperationsRunInArrivalOrder(t *testing.T) {
	client := &fakeClient{}
	server := newTestServer(client, config.Defaults())
	serve(t, server,
		request(t, 1, "complete", uuidParams{UUID: "a"}),
		request(t, 2, "start", uuidParams{UUID: "b"}),
		request(t, 3, "delete", uuidParams{UUID: "c"}),
		request(t, 4, "undo", nil),
	)
	want := []string{"complete a", "start b", "delete c", "undo"}
	if strings.Join(client.actions, ",") != strings.Join(want, ",") {
		t.Fatalf("actions = %v, want %v", client.actions, want)
	}
}

func TestLoadDerivesTodayAndProjectInboxSections(t *testing.T) {
	client := &fakeClient{
		pending: []domain.Task{
			{UUID: "overdue", Description: "Renew cert", Status: "pending", Due: at(2026, 10, 12, 9), Urgency: 5},
			{UUID: "due", Description: "Send invoice", Status: "pending", Due: at(2026, 10, 14, 17), Urgency: 9},
			{UUID: "b", Description: "B task", Status: "pending", Project: "work.client", Entry: at(2026, 10, 1, 9)},
			{UUID: "none", Description: "Loose", Status: "pending", Entry: at(2026, 10, 2, 9)},
			{UUID: "a", Description: "A task", Status: "pending", Project: "home", Entry: at(2026, 10, 3, 9)},
		},
		completed: []domain.Task{{UUID: "done", Description: "Shipped", Status: "completed", End: at(2026, 10, 14, 8)}},
		projects:  []string{"home", "work.client"},
	}
	settings := config.Defaults()
	settings.Projects = domain.ProjectCatalog{{Name: "Work", Value: "work"}, {Name: "Client", Value: "work.client"}}
	responses := serve(t, newTestServer(client, settings), request(t, 1, "load", nil))
	if responses[1].Error != nil {
		t.Fatal(responses[1].Error)
	}
	var result loadResult
	if err := json.Unmarshal(responses[1].Result, &result); err != nil {
		t.Fatal(err)
	}
	titles := func(sections []section) []string {
		values := []string{}
		for _, section := range sections {
			values = append(values, section.Title+"="+strings.Join(section.UUIDs, ","))
		}
		return values
	}
	if got := titles(result.Today); strings.Join(got, ";") != "Overdue=overdue;Due Today=due" {
		t.Fatalf("today = %v", got)
	}
	if got := titles(result.Inbox); strings.Join(got, ";") != "home=a;Work → Client=b;No project=none" {
		t.Fatalf("inbox = %v", got)
	}
	if got := titles(result.Completed); strings.Join(got, ";") != "Today=done" {
		t.Fatalf("completed = %v", got)
	}
	var due taskWire
	for _, task := range result.Tasks {
		if task.UUID == "due" {
			due = task
		}
	}
	if due.TodayGroup != "Due Today" || due.Edit.Due != "2026-10-14 17:00" {
		t.Fatalf("due task = %+v", due)
	}
	values := []string{}
	for _, project := range result.Projects {
		values = append(values, project.Value)
	}
	sort.Strings(values)
	if strings.Join(values, ",") != "home,work,work.client" {
		t.Fatalf("projects = %v", values)
	}
}

func TestModifySendsOnlyChangedFieldsAndReportsFieldErrors(t *testing.T) {
	before := editFields{Description: "Send invoice", Project: "work", Due: "2026-10-14 17:00", Tags: "a b", Estimate: "30m"}
	changed := before
	changed.Priority = "high"
	changed.Tags = "b c"
	changed.Estimate = ""
	changed.Due = "2026-10-15 09:30"

	cases := []struct {
		name  string
		after editFields
		field string
	}{
		{"empty description", editFields{Description: " "}, "description"},
		{"bad priority", editFields{Description: "x", Priority: "urgent"}, "priority"},
		{"bad estimate", editFields{Description: "x", Estimate: "25h"}, "estimate"},
		{"recurrence without due", editFields{Description: "x", Recurrence: "weekly"}, "due"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeClient{}
			responses := serve(t, newTestServer(client, config.Defaults()), request(t, 1, "modify", modifyParams{UUID: "u", Before: before, After: tc.after}))
			if responses[1].Error == nil || responses[1].Error.Field != tc.field || len(client.modified) != 0 {
				t.Fatalf("error = %+v, modified = %v", responses[1].Error, client.modified)
			}
		})
	}

	client := &fakeClient{}
	responses := serve(t, newTestServer(client, config.Defaults()),
		request(t, 1, "modify", modifyParams{UUID: "u", Before: before, After: before}),
		request(t, 2, "modify", modifyParams{UUID: "u", Before: before, After: changed}),
	)
	if string(responses[1].Result) != `{"changed":false}` || string(responses[2].Result) != `{"changed":true}` {
		t.Fatalf("results = %s, %s", responses[1].Result, responses[2].Result)
	}
	if len(client.modified) != 1 {
		t.Fatalf("modified = %v", client.modified)
	}
	diff := client.modified[0]
	if diff.Priority.Value != "H" || diff.Estimate.Kind != domain.Clear || diff.Description.Kind != domain.Unchanged || diff.Due.Value != "2026-10-15 09:30" {
		t.Fatalf("diff = %+v", diff)
	}
	if strings.Join(diff.Tags.Add, ",") != "c" || strings.Join(diff.Tags.Remove, ",") != "a" {
		t.Fatalf("tags = %+v", diff.Tags)
	}
}

func TestCaptureReviewMustResolveFlaggedFields(t *testing.T) {
	client := &fakeClient{}
	server := newTestServer(client, config.Defaults())
	responses := serve(t, server, request(t, 1, "capture", interpretParams{Input: "Call at 3pm"}))
	var captured captureResult
	if err := json.Unmarshal(responses[1].Result, &captured); err != nil {
		t.Fatal(err)
	}
	if captured.Added || captured.Review == nil || captured.Review.Token == 0 || len(client.added) != 0 {
		t.Fatalf("capture = %s", responses[1].Result)
	}

	values := append([]string(nil), captured.Review.ReviewValues...)
	unresolved := serve(t, server, request(t, 2, "captureReview", reviewParams{Token: captured.Review.Token, Values: values}))
	if unresolved[2].Error == nil || len(client.added) != 0 {
		t.Fatalf("unresolved review should be refused: %s", unresolved[2].Result)
	}

	values[0] = "Call"
	values[1] = "tomorrow"
	resolved := reviewParams{Token: captured.Review.Token, Values: values}
	resolved.Touched[0], resolved.Touched[1] = true, true
	added := serve(t, server, request(t, 3, "captureReview", resolved), request(t, 4, "captureReview", resolved))
	if added[3].Error != nil || len(client.added) != 1 || client.added[0].Description != "Call" || client.added[0].Due != "tomorrow" {
		t.Fatalf("review error = %v, added = %+v", added[3].Error, client.added)
	}
	if added[4].Error == nil {
		t.Fatal("a submitted review token must not add twice")
	}
}

func TestCaptureAddsValidInterpretationImmediately(t *testing.T) {
	client := &fakeClient{}
	responses := serve(t, newTestServer(client, config.Defaults()),
		request(t, 1, "capture", interpretParams{Input: "Send invoice #work !high ~30m"}),
		request(t, 2, "captureExplicit", interpretParams{Input: "Call tomorrow at 3pm #home"}),
	)
	if string(responses[1].Result) != `{"added":true}` || responses[2].Error != nil {
		t.Fatalf("responses = %s, %+v", responses[1].Result, responses[2].Error)
	}
	first, second := client.added[0], client.added[1]
	if first.Description != "Send invoice" || first.Project != "work" || first.Priority != "H" || first.Estimate == nil || first.Estimate.Minutes != 30 {
		t.Fatalf("first = %+v", first)
	}
	if second.Description != "Call tomorrow at 3pm" || second.Due != "" || second.Project != "home" {
		t.Fatalf("explicit capture must ignore natural language: %+v", second)
	}
}

func TestSuggestReturnsAppliedInputForEachSuggestion(t *testing.T) {
	client := &fakeClient{projects: []string{"work.client", "home"}}
	server := newTestServer(client, config.Defaults())
	responses := serve(t, server, request(t, 1, "load", nil))
	if responses[1].Error != nil {
		t.Fatal(responses[1].Error)
	}
	responses = serve(t, server, request(t, 2, "suggest", suggestParams{Input: "Fix #wo now", Cursor: 7}))
	var result suggestResult
	if err := json.Unmarshal(responses[2].Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Active || result.Kind != "project" || len(result.Suggestions) == 0 {
		t.Fatalf("suggest = %s", responses[2].Result)
	}
	first := result.Suggestions[0]
	if first.Value != "work.client" || !strings.HasPrefix(first.Input, "Fix #work.client") {
		t.Fatalf("first suggestion = %+v", first)
	}
}

func TestUnknownOperationAndMalformedRequestsReportErrors(t *testing.T) {
	responses := serve(t, newTestServer(&fakeClient{}, config.Defaults()), request(t, 1, "bogus", nil), "{not json")
	if responses[1].Error == nil || !strings.Contains(responses[1].Error.Message, "bogus") {
		t.Fatalf("unknown op = %+v", responses[1])
	}
	if responses[0].Error == nil || !strings.Contains(responses[0].Error.Message, "invalid request") {
		t.Fatalf("malformed = %+v", responses[0])
	}
}
