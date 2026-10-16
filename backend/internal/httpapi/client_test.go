package httpapi

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// TestCoreClientReachesEveryRoute drives the server through the client the
// apps use, so the two cannot disagree about the contract.
func TestCoreClientReachesEveryRoute(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	client := api.NewClient(h.url, h.signIn("alice@example.com").Token)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	version, err := client.Version(ctx)
	must(err)
	session, err := client.Session(ctx)
	must(err)
	zone, err := client.SetTimeZone(ctx, "Europe/Lisbon")
	must(err)
	if version.Server != "v1.2.3" || session.Email != "alice@example.com" || zone.TimeZone != "Europe/Lisbon" {
		t.Errorf("version %+v, session %+v, zone %+v", version, session, zone)
	}

	due := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	instance, err := client.CreateTask(ctx, api.CreateTaskRequest{Description: "Journal", Due: &due, Recurrence: "daily"})
	must(err)
	instance, err = client.ModifyTask(ctx, instance.UUID, api.ModifyTaskRequest{
		EstimateMinutes: api.Change[int]{Kind: domain.Set, Value: 15}, Tags: &api.TagChange{Add: []string{"habit"}},
	})
	must(err)
	_, err = client.StartTask(ctx, instance.UUID)
	must(err)
	_, err = client.StopTask(ctx, instance.UUID)
	must(err)
	completed, err := client.CompleteTask(ctx, instance.UUID)
	must(err)
	pending, err := client.ListTasks(ctx, api.StatusPending, time.Time{})
	must(err)
	if completed.Status != api.StatusCompleted || len(pending) != 1 || pending[0].Parent != instance.Parent || pending[0].UUID == instance.UUID {
		t.Fatalf("after complete: %+v, pending %+v", completed, pending)
	}
	undone, err := client.Undo(ctx)
	must(err)
	pending, err = client.ListTasks(ctx, api.StatusPending, time.Time{})
	must(err)
	if undone.UUID != instance.UUID || undone.Status != api.StatusPending || len(pending) != 1 || pending[0].UUID != instance.UUID ||
		pending[0].EstimateMinutes != 15 || !slices.Equal(pending[0].Tags, []string{"habit"}) {
		t.Fatalf("after undo: %+v, pending %+v", undone, pending)
	}
	template, err := client.StopRecurrence(ctx, instance.UUID)
	must(err)
	recurring, err := client.ListTasks(ctx, api.StatusRecurring, time.Time{})
	must(err)
	if template.UUID != instance.Parent || template.Until == nil || len(recurring) != 1 || recurring[0].UUID != template.UUID {
		t.Errorf("stopped template %+v, recurring %+v", template, recurring)
	}

	report, err := client.CreateTask(ctx, api.CreateTaskRequest{Description: "Report", Project: "work", Tags: []string{"next"}})
	must(err)
	projects, err := client.Projects(ctx)
	must(err)
	tags, err := client.Tags(ctx)
	must(err)
	if !slices.Equal(projects, []string{"work"}) || !slices.Equal(tags, []string{"habit", "next"}) {
		t.Errorf("projects %v, tags %v", projects, tags)
	}
	migration := api.ProjectMigration{Source: "work", Destination: "job"}
	preview, err := client.PreviewProjectMigration(ctx, migration)
	must(err)
	result, err := client.ApplyProjectMigration(ctx, api.ApplyProjectMigrationRequest{ProjectMigration: migration, Tasks: preview.Tasks})
	must(err)
	if len(preview.Tasks) != 1 || preview.Tasks[0].UUID != report.UUID || result.Changed != 1 {
		t.Errorf("preview %+v, result %+v", preview, result)
	}

	before := time.Now().Add(-time.Minute)
	_, err = client.DeleteTask(ctx, report.UUID)
	must(err)
	_, err = client.CompleteTask(ctx, instance.UUID)
	must(err)
	done, err := client.ListTasks(ctx, api.StatusCompleted, before)
	must(err)
	if len(done) != 1 || done[0].UUID != instance.UUID {
		t.Errorf("completed since %v: %+v", before, done)
	}

	for _, test := range []struct {
		name  string
		send  func() error
		code  string
		field string
	}{
		{"an estimate over a day", func() error {
			_, err := client.CreateTask(ctx, api.CreateTaskRequest{Description: "Long", EstimateMinutes: 2000})
			return err
		}, api.CodeInvalid, "estimateMinutes"},
		{"an unknown task", func() error { _, err := client.StartTask(ctx, "6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10"); return err }, api.CodeNotFound, ""},
		{"a finished task", func() error { _, err := client.StartTask(ctx, report.UUID); return err }, api.CodeConflict, ""},
		{"an unknown time zone", func() error { _, err := client.SetTimeZone(ctx, "Mars/Olympus_Mons"); return err }, api.CodeInvalid, "timeZone"},
		{"a revoked token", func() error { _, err := api.NewClient(h.url, "mtk_unknown").Version(ctx); return err }, api.CodeUnauthorized, ""},
	} {
		var failure *api.Error
		if err := test.send(); !errors.As(err, &failure) || failure.Code != test.code || failure.Field != test.field {
			t.Errorf("%s: error = %#v, want %s on %q", test.name, err, test.code, test.field)
		}
	}
}
