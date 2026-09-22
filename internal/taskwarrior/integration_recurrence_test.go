package taskwarrior

import (
	"context"
	"testing"

	"github.com/jvrviegas/momentum/internal/domain"
)

func TestIntegrationRecurringTaskUsesTaskwarriorTemplateAndStopsByParent(t *testing.T) {
	client, _ := isolatedClient(t)
	if err := client.Add(context.Background(), domain.NewTask{Description: "recurring integration", Due: "tomorrow", Recurrence: "daily"}); err != nil {
		t.Fatal(err)
	}
	tasks, err := client.ExportPending(context.Background())
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks=%#v err=%v", tasks, err)
	}
	instance := tasks[0]
	if instance.Recurrence != "daily" || instance.Parent == "" {
		t.Fatalf("generated instance=%#v", instance)
	}
	if err := client.StopRecurrence(context.Background(), instance.Parent); err != nil {
		t.Fatal(err)
	}
	templates, err := client.ExportRecurring(context.Background())
	if err != nil || len(templates) != 1 || templates[0].Until == nil {
		t.Fatalf("after stop templates=%#v err=%v", templates, err)
	}
	tasks, err = client.ExportPending(context.Background())
	if err != nil || len(tasks) != 1 || tasks[0].Parent != instance.Parent {
		t.Fatalf("after stop tasks=%#v err=%v", tasks, err)
	}
}
