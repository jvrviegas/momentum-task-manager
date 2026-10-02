package taskwarrior

import (
	"context"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func TestIntegrationAbsoluteCaptureDateRoundTripsLocalInstant(t *testing.T) {
	client, _ := isolatedClient(t)
	input := domain.NewTask{Description: "absolute capture", Due: "20260922T150000"}
	task := integrationTask(t, client, input)
	if task.Due == nil {
		t.Fatal("due was not exported")
	}
	got := task.Due.In(time.Local)
	if got.Year() != 2026 || got.Month() != time.September || got.Day() != 22 || got.Hour() != 15 || got.Minute() != 0 {
		t.Fatalf("due=%v", got)
	}
	if err := client.Undo(context.Background()); err != nil {
		t.Fatal(err)
	}
}
