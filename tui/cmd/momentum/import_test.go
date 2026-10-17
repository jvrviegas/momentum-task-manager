package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

const (
	newID       = "1b4e28ba-2fa1-11d2-883f-0016d3cca427"
	presentID   = "2c5f39cb-3fb2-12e3-994f-0127e4ddb538"
	differentID = "3d6f4adc-40c3-13f4-a55f-0238f5eec649"
)

type fakeExporter struct {
	tasks []domain.Task
	err   error
}

func (f fakeExporter) ExportAll(context.Context) ([]domain.Task, error) { return f.tasks, f.err }

func taskwarriorExport(t *testing.T, recur string) fakeExporter {
	t.Helper()
	data := `[{"uuid":"` + newID + `","description":"New","status":"pending","entry":"20261001T080000Z","recur":"` + recur + `","due":"20261001T080000Z"},
{"uuid":"` + presentID + `","description":"Present","status":"completed","entry":"20261001T080000Z","end":"20261002T080000Z","client":"acme"},
{"uuid":"` + differentID + `","description":"Different","status":"deleted","entry":"20261001T080000Z","end":"20261002T080000Z","client":"acme"}]`
	var tasks []domain.Task
	if err := json.Unmarshal([]byte(data), &tasks); err != nil {
		t.Fatal(err)
	}
	return fakeExporter{tasks: tasks}
}

// importServer answers the import routes as a server holding presentID
// unchanged and differentID with another project, and records each request.
type importServer struct {
	mu        sync.Mutex
	requests  []string
	failApply bool
}

func newImportServer(t *testing.T) (*importServer, config.Config) {
	t.Helper()
	fake := &importServer{}
	mux := http.NewServeMux()
	for _, route := range []string{api.RoutePreviewImport, api.RouteApplyImport} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			var request api.ImportRequest
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &request)
			fake.mu.Lock()
			fake.requests = append(fake.requests, r.Pattern+" "+r.Header.Get("Authorization"))
			fail := fake.failApply && r.Pattern == api.RouteApplyImport
			fake.mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusConflict)
				io.WriteString(w, `{"code":"conflict","message":"task 1b4e28ba-2fa1-11d2-883f-0016d3cca427 cannot be imported: its UUID is taken"}`)
				return
			}
			result := api.ImportResult{Created: []string{}, Unchanged: []string{}, Different: []api.ImportDifference{}}
			for _, task := range request.Tasks {
				switch task.UUID {
				case presentID:
					result.Unchanged = append(result.Unchanged, task.UUID)
				case differentID:
					result.Different = append(result.Different, api.ImportDifference{UUID: task.UUID, Fields: []string{"project"}})
				default:
					result.Created = append(result.Created, task.UUID)
				}
			}
			json.NewEncoder(w).Encode(result)
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	credentials := filepath.Join(root, "xdg", "momentum", "credentials.json")
	os.MkdirAll(filepath.Dir(credentials), 0o700)
	saved := `{"url":"` + server.URL + `","token":"mtk_saved","email":"ana@example.com","expiresAt":"2099-01-14T12:00:00Z"}`
	if err := os.WriteFile(credentials, []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := config.Defaults()
	settings.Backend.URL = server.URL
	return fake, settings
}

func (s *importServer) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func TestImportDryRunReportsAndImportsNothing(t *testing.T) {
	server, settings := newImportServer(t)
	var out, errOut bytes.Buffer
	code := importTasks(t.Context(), settings, "/tmp/config.toml", taskwarriorExport(t, "weekly"), true, &out, &errOut)
	want := "Read 3 tasks from Taskwarrior: 1 completed, 1 deleted, 1 pending.\n" +
		"Kept as fields Momentum does not use, with how many tasks have each: client (2).\n" +
		"On " + settings.Backend.URL + ": 1 to create, 1 already there, 1 different.\n" +
		"  " + differentID + " differs in project; it is left as it is.\n" +
		"Dry run: nothing was imported.\n"
	if code != 0 || out.String() != want || errOut.Len() != 0 {
		t.Errorf("code %d\nstdout:\n%s\nwant:\n%s\nstderr: %s", code, out.String(), want, errOut.String())
	}
	if seen := server.seen(); len(seen) != 1 || seen[0] != "POST /v1/import/preview Bearer mtk_saved" {
		t.Errorf("requests %q", seen)
	}
}

func TestImportCreatesWhatTheServerLacks(t *testing.T) {
	server, settings := newImportServer(t)
	var out, errOut bytes.Buffer
	code := importTasks(t.Context(), settings, "/tmp/config.toml", taskwarriorExport(t, "weekly"), false, &out, &errOut)
	if code != 0 || !strings.HasSuffix(out.String(), "Imported 1 task. Taskwarrior was not changed.\n") {
		t.Errorf("code %d stdout %s stderr %s", code, out.String(), errOut.String())
	}
	if seen := server.seen(); len(seen) != 2 || seen[1] != "POST /v1/import/apply Bearer mtk_saved" {
		t.Errorf("requests %q", seen)
	}
}

func TestImportWithNothingNewSendsNoImport(t *testing.T) {
	server, settings := newImportServer(t)
	export := taskwarriorExport(t, "weekly")
	export.tasks = export.tasks[1:]
	var out, errOut bytes.Buffer
	code := importTasks(t.Context(), settings, "/tmp/config.toml", export, false, &out, &errOut)
	if code != 0 || !strings.HasSuffix(out.String(), "Nothing to import.\n") || len(server.seen()) != 1 {
		t.Errorf("code %d stdout %s requests %q", code, out.String(), server.seen())
	}
}

func TestImportReportsAFailedImport(t *testing.T) {
	server, settings := newImportServer(t)
	server.failApply = true
	var out, errOut bytes.Buffer
	code := importTasks(t.Context(), settings, "/tmp/config.toml", taskwarriorExport(t, "weekly"), false, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "its UUID is taken\nTasks imported before the failure stay; run momentum import again to finish.") {
		t.Errorf("code %d stderr %s", code, errOut.String())
	}
}

func TestImportRefusesBeforeReachingTheServer(t *testing.T) {
	server, settings := newImportServer(t)
	for name, c := range map[string]struct {
		settings config.Config
		source   fakeExporter
		want     string
	}{
		"an unsupported rule": {settings, taskwarriorExport(t, "every full moon"), `task ` + newID + ` repeats "every full moon"`},
		"no backend":          {config.Defaults(), taskwarriorExport(t, "weekly"), "momentum: import needs a [backend] url in /tmp/config.toml"},
		"Taskwarrior fails":   {settings, fakeExporter{err: errors.New("task: not found")}, "momentum: read Taskwarrior: task: not found"},
	} {
		var out, errOut bytes.Buffer
		if code := importTasks(t.Context(), c.settings, "/tmp/config.toml", c.source, false, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), c.want) {
			t.Errorf("%s: code %d stderr %q", name, code, errOut.String())
		}
	}
	var out, errOut bytes.Buffer
	if code := importTasks(t.Context(), settings, "/tmp/config.toml", fakeExporter{}, false, &out, &errOut); code != 0 ||
		out.String() != "Taskwarrior has no tasks; nothing to import.\n" {
		t.Errorf("no tasks: code %d stdout %q", code, out.String())
	}
	if seen := server.seen(); len(seen) != 0 {
		t.Errorf("requests %q", seen)
	}
	os.Remove(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "momentum", "credentials.json"))
	out.Reset()
	errOut.Reset()
	if code := importTasks(t.Context(), settings, "/tmp/config.toml", taskwarriorExport(t, "weekly"), false, &out, &errOut); code != 1 ||
		errOut.String() != "momentum: not signed in to "+settings.Backend.URL+"; run momentum login first\n" {
		t.Errorf("not signed in: code %d stderr %q", code, errOut.String())
	}
}

func TestDryRunAppliesOnlyToImport(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"today", "--dry-run"}, &out, &errOut); code != 2 || errOut.String() != "momentum: --dry-run applies only to import\n" {
		t.Errorf("code %d stderr %q", code, errOut.String())
	}
	if command, _, err := extractCommand([]string{"--dry-run", "import"}); err != nil || command != "import" {
		t.Errorf("command %q, %v", command, err)
	}
	out.Reset()
	if run([]string{"--help"}, &out, &errOut); !strings.Contains(out.String(), "momentum import [--dry-run]") {
		t.Errorf("help:\n%s", out.String())
	}
}
