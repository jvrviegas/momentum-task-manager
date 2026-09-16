package taskwarrior

import (
	"context"
	"testing"

	"github.com/jvrviegas/momentum/internal/domain"
)

func TestIntegrationProjectMigrationHonorsActiveContextWithoutChangingIt(t *testing.T) {
	client, _ := isolatedClient(t)
	work := migrationTask(t, client, domain.NewTask{Description: "context work", Project: "work"})
	migrationTask(t, client, domain.NewTask{Description: "context personal", Project: "personal"})
	if _, err := client.run(context.Background(), "context", "context", "define", "work", "project:work"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.run(context.Background(), "context", "context", "work"); err != nil {
		t.Fatal(err)
	}

	export, err := client.ExportPendingInContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if export.Scope.ContextName != "work" || export.Scope.ReadFilter != "project:work" || len(export.Tasks) != 1 || export.Tasks[0].UUID != work.UUID {
		t.Fatalf("context export=%#v", export)
	}
	outcome := client.ApplyProjectTask(context.Background(), export.Scope, domain.ProjectTaskMapping{UUID: work.UUID, OldValue: "work", NewValue: "delivery"})
	if outcome.Kind != ProjectTaskChanged || outcome.Observed == nil || outcome.Observed.Project != "delivery" {
		t.Fatalf("outcome=%#v", outcome)
	}
	name, err := client.Context(context.Background())
	if err != nil || name != "work" {
		t.Fatalf("active context changed to %q: %v", name, err)
	}
}
