package bridge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

func (f *fakeClient) ApplyProjectMigration(context.Context, domain.ProjectCatalogPlan, []domain.ProjectTaskMapping) error {
	return nil
}

// migrationClient records the migrations the server was asked to apply, and
// can fail to read tasks or to apply a migration.
type migrationClient struct {
	fakeClient
	applied   [][]domain.ProjectTaskMapping
	exportErr error
	applyErr  error
	// onApply runs as the server is asked, to see what was saved by then.
	onApply func()
}

func (m *migrationClient) ExportPending(ctx context.Context) ([]domain.Task, error) {
	if m.exportErr != nil {
		return nil, m.exportErr
	}
	return m.fakeClient.ExportPending(ctx)
}

func (m *migrationClient) ApplyProjectMigration(_ context.Context, _ domain.ProjectCatalogPlan, tasks []domain.ProjectTaskMapping) error {
	if m.onApply != nil {
		m.onApply()
	}
	if m.applyErr != nil {
		return m.applyErr
	}
	m.applied = append(m.applied, tasks)
	return nil
}

const projectsConfig = backendTable + `# my notes
[[projects]]
name = "Work"
value = "work"

[[projects]]
name = "Client"
value = "work.client"

[[projects]]
name = "Home"
value = "home"

[[projects]]
name = "Empty"
value = "empty"
`

func pendingTask(uuid, project string) domain.Task {
	return domain.Task{UUID: uuid, Description: "Task " + uuid, Status: "pending", Project: project}
}

// projectServer is a bridge over a config with a small catalog and tasks in
// work, work.client, a lookalike workshop, home and side.gig, which the
// catalog lacks, and a repeating one that a migration leaves alone.
func projectServer(t *testing.T) (*Server, *configFile, *migrationClient) {
	t.Helper()
	file := newConfigFile(t, projectsConfig)
	repeating := pendingTask("6", "work")
	repeating.Recurrence = "weekly"
	client := &migrationClient{fakeClient: fakeClient{pending: []domain.Task{
		pendingTask("1", "work"), pendingTask("2", "work.client"), pendingTask("3", "workshop"), pendingTask("4", "home"),
		pendingTask("5", "side.gig"), repeating,
	}}}
	settings, _, err := config.LoadWithOptions(config.LoadOptions{PathOverride: file.path})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(func(string) Client { return client }, settings, file.path)
	server.now = func() time.Time { return testNow }
	return server, file, client
}

func projectsOf(t *testing.T, server *Server) *projectsResult {
	t.Helper()
	result, err := call(t, server, "projects", nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.(*projectsResult)
}

func savedProject(t *testing.T, server *Server, params projectEditParams) *saveProjectResult {
	t.Helper()
	result, err := call(t, server, "saveProject", params)
	if err != nil {
		t.Fatalf("saveProject: %v", err)
	}
	return result.(*saveProjectResult)
}

func previewOf(t *testing.T, server *Server, params projectEditParams) *projectPreviewWire {
	t.Helper()
	result, err := call(t, server, "previewMigration", params)
	if err != nil {
		t.Fatalf("previewMigration: %v", err)
	}
	return result.(*projectPreviewWire)
}

func uuidsOf(tasks []projectTaskWire) string {
	ids := make([]string, len(tasks))
	for i, task := range tasks {
		ids[i] = task.UUID
	}
	return strings.Join(ids, ",")
}

func TestProjectsListTheCatalogAndTheValuesFoundOnlyOnTasks(t *testing.T) {
	server, _, client := projectServer(t)
	client.pending = append(client.pending, pendingTask("7", "work-extra"))

	got := projectsOf(t, server)
	var rows []string
	for _, entry := range got.Entries {
		rows = append(rows, entry.Value+" "+entry.Name+" "+entry.Tag+" "+string(rune('0'+entry.Tasks)))
	}
	want := "empty Empty catalog 0|home Home catalog 1|side.gig gig tasks 1|work Work catalog 2|work.client Client catalog 1|work-extra work-extra tasks 1|workshop workshop tasks 1"
	if strings.Join(rows, "|") != want {
		t.Fatalf("entries:\n got %s\nwant %s", strings.Join(rows, "|"), want)
	}
	if got.Revision == "" || got.TasksError != "" {
		t.Fatalf("revision=%q error=%q", got.Revision, got.TasksError)
	}
}

func TestProjectsKeepTheCatalogWhenTheTasksCannotBeRead(t *testing.T) {
	server, _, client := projectServer(t)
	client.exportErr = errors.New("the server did not answer")
	got := projectsOf(t, server)
	if got.TasksError != "the server did not answer" || len(got.Entries) != 4 {
		t.Fatalf("result = %+v", got)
	}
	for _, entry := range got.Entries {
		if entry.Tag != "catalog" || entry.Tasks != 0 {
			t.Errorf("entry = %+v", entry)
		}
	}
}

func TestAnInvalidProjectDraftIsRefusedOnItsFieldAndNothingIsWritten(t *testing.T) {
	server, file, client := projectServer(t)
	revision := projectsOf(t, server).Revision
	before := contents(t, file.path)

	for name, test := range map[string]struct {
		params projectEditParams
		field  string
	}{
		"no name":                  {projectEditParams{Action: "add", Segment: "acme"}, "name"},
		"a segment with capitals":  {projectEditParams{Action: "add", Name: "Acme", Segment: "Acme Co"}, "segment"},
		"an empty segment":         {projectEditParams{Action: "add", Name: "Acme"}, "segment"},
		"a parent that is no path": {projectEditParams{Action: "add", Name: "Acme", Parent: "Work!", Segment: "acme"}, "parent"},
		"a value already there":    {projectEditParams{Action: "add", Name: "Again", Parent: "work", Segment: "client"}, "segment"},
		"a rename onto another":    {projectEditParams{Action: "edit", Source: "work.client", Name: "Home", Segment: "home"}, "segment"},
		"a move under itself":      {projectEditParams{Action: "edit", Source: "work", Name: "Work", Parent: "work.client", Segment: "work"}, "parent"},
	} {
		test.params.Revision = revision
		refusal := refused(t, server, "saveProject", test.params)
		if refusal.Field != test.field || refusal.Message == "" {
			t.Errorf("%s: refusal=%+v, want field %q", name, refusal, test.field)
		}
		if contents(t, file.path) != before || len(client.applied) != 0 {
			t.Fatalf("%s: nothing may be written", name)
		}
	}
}

func TestAddingAnEntryComposesItsValueAndKeepsTheFilesComments(t *testing.T) {
	server, file, _ := projectServer(t)
	revision := projectsOf(t, server).Revision

	got := savedProject(t, server, projectEditParams{Revision: revision, Action: "add", Name: "Acme", Parent: "work.client", Segment: "acme"})
	if got.Revision == revision || got.Changed != 0 {
		t.Fatalf("result = %+v", got)
	}
	text := contents(t, file.path)
	if !strings.Contains(text, `value = "work.client.acme"`) || !strings.Contains(text, "# my notes") {
		t.Fatalf("the file should hold the entry and its comment:\n%s", text)
	}
	var found bool
	for _, entry := range projectsOf(t, server).Entries {
		found = found || (entry.Value == "work.client.acme" && entry.Name == "Acme" && entry.Tag == "catalog")
	}
	if !found {
		t.Fatal("the new entry is in use at once")
	}
}

func TestChangingOnlyTheNameNeverOffersOrMovesATask(t *testing.T) {
	server, file, client := projectServer(t)
	revision := projectsOf(t, server).Revision
	edit := projectEditParams{Revision: revision, Action: "edit", Source: "home", Name: "House", Segment: "home"}

	if preview := previewOf(t, server, edit); preview.ValueChanged || len(preview.ExactTasks) != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	edit.Migrate = true // even asked to, there is no value change to move tasks for
	savedProject(t, server, edit)
	if text := contents(t, file.path); !strings.Contains(text, `name = "House"`) || !strings.Contains(text, `value = "home"`) {
		t.Fatalf("the file should hold the new name:\n%s", text)
	}
	if len(client.applied) != 0 {
		t.Fatal("no task may move")
	}
}

func TestAPreviewSaysWhichPendingTasksARenameMoves(t *testing.T) {
	server, file, _ := projectServer(t)
	before := contents(t, file.path)
	revision := projectsOf(t, server).Revision

	preview := previewOf(t, server, projectEditParams{Revision: revision, Action: "edit", Source: "work", Name: "Job", Segment: "job"})
	if !preview.ValueChanged || preview.Value != "job" || preview.CatalogDescendants != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	if got := uuidsOf(preview.ExactTasks); got != "1" {
		t.Errorf("exact tasks = %s, want only the pending task in work: not work.client, not workshop, not the repeating one", got)
	}
	if got := uuidsOf(preview.SubprojectTasks); got != "1,2" {
		t.Errorf("subproject tasks = %s", got)
	}
	if preview.SubprojectTasks[1].OldValue != "work.client" || preview.SubprojectTasks[1].NewValue != "job.client" {
		t.Errorf("a subproject keeps its suffix: %+v", preview.SubprojectTasks[1])
	}
	if contents(t, file.path) != before {
		t.Fatal("a preview writes nothing")
	}
}

func TestAValueTasksAlreadyUseIsAMergeWarning(t *testing.T) {
	server, _, _ := projectServer(t)
	revision := projectsOf(t, server).Revision
	preview := previewOf(t, server, projectEditParams{Revision: revision, Action: "edit", Source: "home", Name: "Gig", Segment: "side-gig"})
	if preview.DestinationTaskOnly {
		t.Fatalf("side-gig is on no task: %+v", preview)
	}
	preview = previewOf(t, server, projectEditParams{Revision: revision, Action: "edit", Source: "home", Name: "Gig", Parent: "side", Segment: "gig"})
	if !preview.DestinationTaskOnly {
		t.Fatalf("side.gig is on a task but not in the catalog: %+v", preview)
	}
}

func TestACatalogOnlyRenameMovesNoTask(t *testing.T) {
	server, file, client := projectServer(t)
	revision := projectsOf(t, server).Revision
	got := savedProject(t, server, projectEditParams{Revision: revision, Action: "edit", Source: "work", Name: "Job", Segment: "job"})
	if got.Changed != 0 || len(client.applied) != 0 {
		t.Fatalf("result = %+v", got)
	}
	text := contents(t, file.path)
	if !strings.Contains(text, `value = "job"`) || !strings.Contains(text, `value = "work.client"`) {
		t.Fatalf("only the entry moves, not its children:\n%s", text)
	}
}

func TestIncludingSubprojectsRenamesTheCatalogDescendantsAndMovesTheirTasks(t *testing.T) {
	server, file, client := projectServer(t)
	revision := projectsOf(t, server).Revision
	edit := projectEditParams{Revision: revision, Action: "edit", Source: "work", Name: "Job", Segment: "job", IncludeSubprojects: true}
	edit.Tasks = previewOf(t, server, edit).SubprojectTasks
	edit.Migrate = true

	got := savedProject(t, server, edit)
	if got.Changed != 2 || got.TaskError != "" || len(client.applied) != 1 || len(client.applied[0]) != 2 {
		t.Fatalf("result = %+v applied=%v", got, client.applied)
	}
	if text := contents(t, file.path); !strings.Contains(text, `value = "job.client"`) || strings.Contains(text, `"work`) {
		t.Fatalf("the descendant follows its parent:\n%s", text)
	}
}

func TestTheCatalogIsSavedBeforeTheTasksMove(t *testing.T) {
	server, file, client := projectServer(t)
	revision := projectsOf(t, server).Revision
	edit := projectEditParams{Revision: revision, Action: "edit", Source: "work", Name: "Job", Segment: "job"}
	edit.Tasks = previewOf(t, server, edit).ExactTasks
	edit.Migrate = true
	var savedFirst bool
	client.onApply = func() { savedFirst = strings.Contains(contents(t, file.path), `value = "job"`) }

	if got := savedProject(t, server, edit); got.Changed != 1 {
		t.Fatalf("result = %+v", got)
	}
	if !savedFirst {
		t.Fatal("the catalog must be on disk when the server is asked")
	}
}

func TestARefusedMigrationKeepsTheSavedCatalog(t *testing.T) {
	for name, test := range map[string]struct {
		err   error
		stale bool
	}{
		"the server refuses the tasks": {&api.Error{Code: api.CodeConflict, Message: "tasks changed"}, true},
		"the server cannot be reached": {errors.New("the server did not answer"), false},
	} {
		server, file, client := projectServer(t)
		revision := projectsOf(t, server).Revision
		edit := projectEditParams{Revision: revision, Action: "edit", Source: "work", Name: "Job", Segment: "job"}
		edit.Tasks = previewOf(t, server, edit).ExactTasks
		edit.Migrate = true
		client.applyErr = test.err

		got := savedProject(t, server, edit)
		if got.Changed != 0 || got.TaskError == "" || got.Stale != test.stale {
			t.Errorf("%s: result = %+v", name, got)
		}
		if !strings.Contains(contents(t, file.path), `value = "job"`) {
			t.Errorf("%s: the saved catalog stays", name)
		}
	}
}

func TestAStalePreviewIsRefusedAndNothingIsWritten(t *testing.T) {
	server, file, client := projectServer(t)
	revision := projectsOf(t, server).Revision
	edit := projectEditParams{Revision: revision, Action: "edit", Source: "work", Name: "Job", Segment: "job"}
	edit.Tasks = previewOf(t, server, edit).ExactTasks
	edit.Migrate = true
	before := contents(t, file.path)

	client.pending = append(client.pending, pendingTask("8", "work")) // a task arrives after the preview
	refusal := refused(t, server, "saveProject", edit)
	if refusal.Code != CodeStalePreview {
		t.Fatalf("refusal = %+v", refusal)
	}
	if contents(t, file.path) != before || len(client.applied) != 0 {
		t.Fatal("a stale preview changes neither the catalog nor a task")
	}
}

func TestARenameWithNoMatchingTasksIsAValidCatalogSave(t *testing.T) {
	server, file, client := projectServer(t)
	revision := projectsOf(t, server).Revision
	edit := projectEditParams{Revision: revision, Action: "edit", Source: "empty", Name: "Void", Segment: "void", Migrate: true}
	edit.Tasks = previewOf(t, server, edit).ExactTasks

	got := savedProject(t, server, edit)
	if got.Changed != 0 || got.TaskError != "" || len(client.applied) != 0 {
		t.Fatalf("result = %+v", got)
	}
	if !strings.Contains(contents(t, file.path), `value = "void"`) {
		t.Fatal("the catalog is saved")
	}
}

func TestRemovingWithAndWithoutChildrenLeavesTheCatalogAsChosen(t *testing.T) {
	for _, withChildren := range []bool{false, true} {
		server, file, client := projectServer(t)
		revision := projectsOf(t, server).Revision
		savedProject(t, server, projectEditParams{Revision: revision, Action: "remove", Source: "work", RemoveChildren: withChildren})

		text := contents(t, file.path)
		if strings.Contains(text, `value = "work"`) {
			t.Errorf("withChildren=%v: work should be gone:\n%s", withChildren, text)
		}
		if has := strings.Contains(text, `value = "work.client"`); has == withChildren {
			t.Errorf("withChildren=%v: work.client present=%v", withChildren, has)
		}
		if !strings.Contains(text, `value = "home"`) || !strings.Contains(text, "# my notes") {
			t.Errorf("withChildren=%v: the rest stays:\n%s", withChildren, text)
		}
		if len(client.applied) != 0 || len(client.pending) != 6 {
			t.Errorf("withChildren=%v: removing never changes a task", withChildren)
		}
	}
}

func TestAProjectSaveOnAFileThatChangedElsewhereIsRefused(t *testing.T) {
	server, file, _ := projectServer(t)
	revision := projectsOf(t, server).Revision
	file.write(projectsConfig + "# added in an editor\n")
	elsewhere := contents(t, file.path)

	refusal := refused(t, server, "saveProject", projectEditParams{Revision: revision, Action: "add", Name: "Acme", Segment: "acme"})
	if refusal.Code != CodeConfigConflict {
		t.Fatalf("refusal = %+v", refusal)
	}
	if contents(t, file.path) != elsewhere {
		t.Fatal("a refused save leaves the file as it is")
	}
}
