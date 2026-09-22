package taskwarrior

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum/internal/domain"
)

// isolatedEnvironment is mandatory for every real Taskwarrior integration
// test. It is created before the first client command and rejects the user's
// default paths before TASKRC/TASKDATA are installed.
type isolatedEnvironment struct {
	Root    string
	TaskRC  string
	DataDir string
}

func newIsolatedEnvironment(t *testing.T) isolatedEnvironment {
	t.Helper()
	if _, err := exec.LookPath("task"); err != nil {
		t.Skip("Taskwarrior 3.x binary unavailable; isolated integration skipped")
	}
	root := t.TempDir()
	env := isolatedEnvironment{Root: root, TaskRC: filepath.Join(root, "taskrc"), DataDir: filepath.Join(root, "data")}
	if err := os.MkdirAll(env.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{env.TaskRC, env.DataDir} {
		if filepath.Clean(path) == filepath.Join(home, ".taskrc") || filepath.Clean(path) == filepath.Join(home, ".task") {
			t.Fatalf("integration guard rejected production Taskwarrior path %q", path)
		}
	}
	config := "data.location=" + env.DataDir + "\nconfirmation=no\nhooks=off\nverbose=no\n"
	if err := os.WriteFile(env.TaskRC, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	// t.Setenv restores both process variables after this test. No test in this
	// file is parallel because Taskwarrior reads process environment.
	t.Setenv("TASKRC", env.TaskRC)
	t.Setenv("TASKDATA", env.DataDir)
	if got := os.Getenv("TASKRC"); filepath.Clean(got) == filepath.Join(home, ".taskrc") {
		t.Fatal("integration guard detected real TASKRC after setup")
	}
	return env
}

func isolatedClient(t *testing.T) (*CommandClient, isolatedEnvironment) {
	t.Helper()
	env := newIsolatedEnvironment(t)
	return NewClient("task"), env
}

func enableEstimateUDA(t *testing.T, env isolatedEnvironment) {
	t.Helper()
	file, err := os.OpenFile(env.TaskRC, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("uda.estimate.type=duration\nuda.estimate.label=Estimate\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func integrationTask(t *testing.T, client *CommandClient, input domain.NewTask) domain.Task {
	t.Helper()
	if err := client.Add(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	tasks, err := client.ExportPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one task, got %d", len(tasks))
	}
	return tasks[0]
}

func TestIntegrationAddAndExport(t *testing.T) {
	client, _ := isolatedClient(t)
	task := integrationTask(t, client, domain.NewTask{Description: "integration add", Project: "work.client", Priority: "H", Due: "tomorrow", Scheduled: "today", Tags: []string{"planning", "client"}})
	if task.UUID == "" || task.Description != "integration add" || task.Project != "work.client" || task.Priority != "H" {
		t.Fatalf("decoded task=%#v", task)
	}
	if !contains(task.Tags, "planning") || !contains(task.Tags, "client") {
		t.Fatalf("tags=%#v", task.Tags)
	}
}

func TestIntegrationNextWeekAliasSchedulesFridayOfNextCalendarWeek(t *testing.T) {
	client, _ := isolatedClient(t)
	now := time.Now().In(time.Local)
	daysSinceMonday := (int(now.Weekday()) - int(time.Monday) + 7) % 7
	want := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).
		AddDate(0, 0, -daysSinceMonday+11)

	task := integrationTask(t, client, domain.NewTask{Description: "next week Friday", Scheduled: "next-week"})
	if task.Scheduled == nil {
		t.Fatal("scheduled date is absent")
	}
	got := task.Scheduled.In(time.Local)
	if got.Year() != want.Year() || got.YearDay() != want.YearDay() || got.Weekday() != time.Friday {
		t.Fatalf("scheduled=%v want next-week Friday %v", got, want)
	}
}

func TestIntegrationEstimateAddModifyClearAndUndo(t *testing.T) {
	client, env := isolatedClient(t)
	enableEstimateUDA(t, env)
	oneHour := domain.Estimate{Minutes: 60}
	task := integrationTask(t, client, domain.NewTask{Description: "estimate lifecycle", Estimate: &oneHour})
	if task.Estimate == nil || task.Estimate.Minutes != 60 {
		t.Fatalf("initial estimate=%#v raw=%#v", task.Estimate, task.RawFields["estimate"])
	}

	ninetyMinutes := &domain.Estimate{Minutes: 90}
	if err := client.Modify(context.Background(), task.UUID, domain.TaskDiff{Estimate: domain.EstimateChange{Kind: domain.Set, Value: ninetyMinutes}}); err != nil {
		t.Fatal(err)
	}
	modified, err := client.ExportPending(context.Background())
	if err != nil || len(modified) != 1 || modified[0].Estimate == nil || modified[0].Estimate.Minutes != 90 {
		t.Fatalf("modified=%#v err=%v", modified, err)
	}
	if err := client.Modify(context.Background(), task.UUID, domain.TaskDiff{Estimate: domain.EstimateChange{Kind: domain.Clear}}); err != nil {
		t.Fatal(err)
	}
	cleared, err := client.ExportPending(context.Background())
	if err != nil || len(cleared) != 1 || cleared[0].Estimate != nil {
		t.Fatalf("cleared=%#v err=%v", cleared, err)
	}
	if err := client.Undo(context.Background()); err != nil {
		t.Fatal(err)
	}
	restored, err := client.ExportPending(context.Background())
	if err != nil || len(restored) != 1 || restored[0].Estimate == nil || restored[0].Estimate.Minutes != 90 {
		t.Fatalf("restored=%#v err=%v", restored, err)
	}
}

func TestIntegrationEstimateMutationRefusesMissingUDAWithoutChangingTaskData(t *testing.T) {
	client, _ := isolatedClient(t)
	estimate := domain.Estimate{Minutes: 60}
	err := client.Add(context.Background(), domain.NewTask{Description: "must not be created", Estimate: &estimate})
	var readinessErr *EstimateUDAError
	if !errors.As(err, &readinessErr) || readinessErr.State != EstimateUDAMissing {
		t.Fatalf("err=%T %v", err, err)
	}
	tasks, exportErr := client.ExportPending(context.Background())
	if exportErr != nil || len(tasks) != 0 {
		t.Fatalf("tasks=%#v exportErr=%v", tasks, exportErr)
	}
}

func TestIntegrationModifyByUUIDAndClearFields(t *testing.T) {
	client, _ := isolatedClient(t)
	task := integrationTask(t, client, domain.NewTask{Description: "before", Project: "work", Due: "tomorrow", Tags: []string{"old"}})
	diff := domain.TaskDiff{
		Description: domain.FieldChange{Kind: domain.Set, Value: "after"},
		Project:     domain.FieldChange{Kind: domain.Clear},
		Due:         domain.FieldChange{Kind: domain.Clear},
		Tags:        domain.TagChange{Changed: true, Remove: []string{"old"}, Add: []string{"new"}},
	}
	if err := client.Modify(context.Background(), task.UUID, diff); err != nil {
		t.Fatal(err)
	}
	got, err := client.ExportPending(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatalf("tasks=%#v err=%v", got, err)
	}
	if got[0].Description != "after" || got[0].Project != "" || got[0].Due != nil || !contains(got[0].Tags, "new") || contains(got[0].Tags, "old") {
		t.Fatalf("modified task=%#v", got[0])
	}
}

func TestIntegrationStartStopDoneLifecycle(t *testing.T) {
	client, _ := isolatedClient(t)
	task := integrationTask(t, client, domain.NewTask{Description: "lifecycle"})
	if err := client.Start(context.Background(), task.UUID); err != nil {
		t.Fatal(err)
	}
	started, err := client.ExportPending(context.Background())
	if err != nil || len(started) != 1 || started[0].Start == nil {
		t.Fatalf("started=%#v err=%v", started, err)
	}
	if err := client.Stop(context.Background(), task.UUID); err != nil {
		t.Fatal(err)
	}
	stopped, err := client.ExportPending(context.Background())
	if err != nil || len(stopped) != 1 || stopped[0].Start != nil {
		t.Fatalf("stopped=%#v err=%v", stopped, err)
	}
	if err := client.Complete(context.Background(), task.UUID); err != nil {
		t.Fatal(err)
	}
	pending, err := client.ExportPending(context.Background())
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending=%#v err=%v", pending, err)
	}
}

func TestIntegrationDeleteAndUndo(t *testing.T) {
	client, _ := isolatedClient(t)
	task := integrationTask(t, client, domain.NewTask{Description: "delete me"})
	if err := client.Delete(context.Background(), task.UUID); err != nil {
		t.Fatal(err)
	}
	pending, err := client.ExportPending(context.Background())
	if err != nil || len(pending) != 0 {
		t.Fatalf("after delete=%#v err=%v", pending, err)
	}
	if err := client.Undo(context.Background()); err != nil {
		t.Fatal(err)
	}
	restored, err := client.ExportPending(context.Background())
	if err != nil || len(restored) != 1 || restored[0].UUID != task.UUID {
		t.Fatalf("after undo=%#v err=%v", restored, err)
	}
}

func TestIntegrationContextAndDiscovery(t *testing.T) {
	client, _ := isolatedClient(t)
	integrationTask(t, client, domain.NewTask{Description: "projected", Project: "project.one", Tags: []string{"tag.one"}})
	contextName, err := client.Context(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if contextName != "" {
		t.Fatalf("unexpected default context=%q", contextName)
	}
	projects, err := client.Projects(context.Background())
	if err != nil || !contains(projects, "project.one") {
		t.Fatalf("projects=%#v err=%v", projects, err)
	}
	tags, err := client.Tags(context.Background())
	if err != nil || !contains(tags, "tag.one") {
		t.Fatalf("tags=%#v err=%v", tags, err)
	}
}

func TestIntegrationValidationFailureIsTypedAndIsolated(t *testing.T) {
	client, env := isolatedClient(t)
	err := client.Add(context.Background(), domain.NewTask{Description: "bad date", Due: "not-a-taskwarrior-date"})
	var commandErr *CommandError
	if !errors.As(err, &commandErr) || commandErr.ExitCode == 0 || !strings.Contains(err.Error(), "date") {
		t.Fatalf("err=%T %v", err, err)
	}
	if filepath.Clean(os.Getenv("TASKRC")) != filepath.Clean(env.TaskRC) || filepath.Clean(os.Getenv("TASKDATA")) != filepath.Clean(env.DataDir) {
		t.Fatal("integration environment changed unexpectedly")
	}
}

func TestIntegrationNoRealDatabasePathCanBeSelected(t *testing.T) {
	_, env := isolatedClient(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(env.TaskRC) == filepath.Join(home, ".taskrc") || filepath.Clean(env.DataDir) == filepath.Join(home, ".task") {
		t.Fatal("real database path selected")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
