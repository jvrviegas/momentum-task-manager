package taskwarrior

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func migrationTask(t *testing.T, client *CommandClient, input domain.NewTask) domain.Task {
	t.Helper()
	if err := client.Add(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	export, err := client.ExportPendingInContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range export.Tasks {
		if task.Description == input.Description {
			return task
		}
	}
	t.Fatalf("added task %q was not exported", input.Description)
	return domain.Task{}
}

func TestIntegrationProjectMigrationIsPendingExactAndContextScoped(t *testing.T) {
	client, _ := isolatedClient(t)
	root := migrationTask(t, client, domain.NewTask{Description: "root", Project: "work", Tags: []string{"keep"}})
	child := migrationTask(t, client, domain.NewTask{Description: "child", Project: "work.client"})
	lookalike := migrationTask(t, client, domain.NewTask{Description: "lookalike", Project: "workshop"})
	unrelated := migrationTask(t, client, domain.NewTask{Description: "unrelated", Project: "personal"})
	completed := migrationTask(t, client, domain.NewTask{Description: "completed", Project: "work"})
	if err := client.Complete(context.Background(), completed.UUID); err != nil {
		t.Fatal(err)
	}

	export, err := client.ExportPendingInContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	mappings := domain.MapPendingProjectTasks(export.Tasks, "work", "delivery", true)
	if len(mappings) != 2 {
		t.Fatalf("mappings=%#v tasks=%#v", mappings, export.Tasks)
	}
	for _, mapping := range mappings {
		outcome := client.ApplyProjectTask(context.Background(), export.Scope, mapping)
		if outcome.Kind != ProjectTaskChanged || outcome.Observed == nil || outcome.Observed.Project != mapping.NewValue {
			t.Fatalf("mapping=%#v outcome=%#v", mapping, outcome)
		}
	}

	got, err := client.ExportPendingInContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byUUID := make(map[string]domain.Task, len(got.Tasks))
	for _, task := range got.Tasks {
		byUUID[task.UUID] = task
	}
	if byUUID[root.UUID].Project != "delivery" || byUUID[child.UUID].Project != "delivery.client" || byUUID[lookalike.UUID].Project != "workshop" || byUUID[unrelated.UUID].Project != "personal" {
		t.Fatalf("pending projects=%#v", byUUID)
	}
	if byUUID[root.UUID].Description != "root" || len(byUUID[root.UUID].Tags) != 1 || byUUID[root.UUID].Tags[0] != "keep" {
		t.Fatalf("unrelated task fields changed=%#v", byUUID[root.UUID])
	}

	stale := migrationTask(t, client, domain.NewTask{Description: "stale", Project: "work"})
	staleMapping := domain.ProjectTaskMapping{UUID: stale.UUID, OldValue: "work", NewValue: "delivery"}
	if err := client.Complete(context.Background(), stale.UUID); err != nil {
		t.Fatal(err)
	}
	outcome := client.ApplyProjectTask(context.Background(), export.Scope, staleMapping)
	if outcome.Kind != ProjectTaskSkipped || outcome.Observed == nil || outcome.Observed.Status != "completed" || outcome.Observed.Project != "work" {
		t.Fatalf("stale outcome=%#v", outcome)
	}
}

func TestIntegrationProjectMigrationReportsHookRejection(t *testing.T) {
	client, env := isolatedClient(t)
	task := migrationTask(t, client, domain.NewTask{Description: "hooked", Project: "work"})
	hookDir := filepath.Join(env.DataDir, "hooks")
	if err := os.MkdirAll(hookDir, 0o700); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(hookDir, "on-modify")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ncat >/dev/null\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configData, err := os.ReadFile(env.TaskRC)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.TaskRC, append(configData, []byte("hooks=on\n")...), 0o600); err != nil {
		t.Fatal(err)
	}

	export, err := client.ExportPendingInContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	outcome := client.ApplyProjectTask(context.Background(), export.Scope, domain.ProjectTaskMapping{UUID: task.UUID, OldValue: "work", NewValue: "delivery"})
	if outcome.Kind != ProjectTaskFailed || outcome.Observed == nil || outcome.Observed.Project != "work" || outcome.Err == nil {
		t.Fatalf("hook outcome=%#v", outcome)
	}
}
