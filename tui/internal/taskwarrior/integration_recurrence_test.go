package taskwarrior

import (
	"context"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

func TestIntegrationRecurrencePreviewMatchesGeneratedOccurrences(t *testing.T) {
	lisbon, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		t.Fatal(err)
	}
	baseYear := time.Now().In(lisbon).Year() + 2
	leapYear := baseYear
	for time.Date(leapYear, time.February, 29, 0, 0, 0, 0, lisbon).Month() != time.February {
		leapYear++
	}
	marchLast := time.Date(baseYear, time.April, 0, 15, 0, 0, 0, lisbon)
	for marchLast.Weekday() != time.Sunday {
		marchLast = marchLast.AddDate(0, 0, -1)
	}
	friday := time.Date(baseYear, time.September, 1, 15, 0, 0, 0, lisbon)
	for friday.Weekday() != time.Friday {
		friday = friday.AddDate(0, 0, 1)
	}
	tests := []struct {
		name       string
		anchor     time.Time
		expression string
	}{
		{name: "daily across DST", anchor: marchLast.AddDate(0, 0, -1), expression: "daily"},
		{name: "weekdays across weekend", anchor: friday, expression: "weekdays"},
		{name: "weekly across DST", anchor: marchLast.AddDate(0, 0, -1), expression: "weekly"},
		{name: "monthly across DST", anchor: marchLast.AddDate(0, 0, -1), expression: "monthly"},
		{name: "monthly month end", anchor: time.Date(baseYear, time.January, 31, 15, 0, 0, 0, lisbon), expression: "monthly"},
		{name: "monthly leap year", anchor: time.Date(leapYear, time.January, 31, 15, 0, 0, 0, lisbon), expression: "monthly"},
		{name: "day interval", anchor: marchLast.AddDate(0, 0, -1), expression: "2days"},
		{name: "week interval", anchor: marchLast.AddDate(0, 0, -1), expression: "2wks"},
		{name: "month interval", anchor: time.Date(baseYear, time.December, 31, 15, 0, 0, 0, lisbon), expression: "2mo"},
		{name: "quarterly interval", anchor: time.Date(baseYear, time.January, 31, 15, 0, 0, 0, lisbon), expression: "quarterly"},
		{name: "semiannual interval", anchor: time.Date(baseYear, time.January, 31, 15, 0, 0, 0, lisbon), expression: "semiannual"},
		{name: "annual leap day", anchor: time.Date(leapYear, time.February, 29, 15, 0, 0, 0, lisbon), expression: "annual"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TZ", "Europe/Lisbon")
			client, env := isolatedClient(t)
			enableRecurrenceLimit(t, env, 2)
			if err := client.Add(context.Background(), domain.NewTask{
				Description: "recurrence preview comparison",
				Due:         test.anchor.Format("20060102T150405"),
				Recurrence:  test.expression,
			}); err != nil {
				t.Fatal(err)
			}
			tasks, err := client.ExportPending(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(tasks) < 2 {
				t.Fatalf("generated tasks=%#v", tasks)
			}
			for _, task := range tasks {
				if task.Due == nil {
					t.Fatalf("generated task has no due date: %#v", task)
				}
			}
			sort.Slice(tasks, func(i, j int) bool { return tasks[i].Due.Before(*tasks[j].Due) })
			preview, ok := domain.RecurrenceNext(test.anchor, test.expression)
			if !ok || !preview.Equal(*tasks[1].Due) {
				t.Fatalf("preview=%v ok=%v; Taskwarrior next=%v (first=%v)", preview, ok, tasks[1].Due, tasks[0].Due)
			}
		})
	}
}

// TestIntegrationNextInstanceDueDepartsFromTaskwarriorOnlyAsDocumented
// compares four occurrences Taskwarrior generates with the backend's series
// (backend plan D5, T11). They agree except where the backend keeps the wall
// clock across a DST change or counts calendar months from the anchor; those
// cases must differ, so the documented departures stay true.
func TestIntegrationNextInstanceDueDepartsFromTaskwarriorOnlyAsDocumented(t *testing.T) {
	lisbon, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		t.Fatal(err)
	}
	baseYear := time.Now().In(lisbon).Year() + 2
	leapYear := baseYear
	for time.Date(leapYear, time.February, 29, 0, 0, 0, 0, lisbon).Month() != time.February {
		leapYear++
	}
	marchLast := time.Date(baseYear, time.April, 0, 9, 0, 0, 0, lisbon)
	for marchLast.Weekday() != time.Sunday {
		marchLast = marchLast.AddDate(0, 0, -1)
	}
	friday := time.Date(baseYear, time.September, 1, 15, 0, 0, 0, lisbon)
	for friday.Weekday() != time.Friday {
		friday = friday.AddDate(0, 0, 1)
	}
	july := time.Date(baseYear, time.July, 1, 9, 0, 0, 0, lisbon)
	tests := []struct {
		name       string
		anchor     time.Time
		expression string
		same       bool
	}{
		{name: "daily", anchor: july, expression: "daily", same: true},
		{name: "two weeks", anchor: july, expression: "2wks", same: true},
		{name: "weekdays across a weekend", anchor: friday, expression: "weekdays", same: true},
		{name: "monthly across DST", anchor: time.Date(baseYear, time.January, 15, 9, 0, 0, 0, lisbon), expression: "monthly", same: true},
		{name: "annual leap day", anchor: time.Date(leapYear, time.February, 29, 9, 0, 0, 0, lisbon), expression: "annual", same: true},
		{name: "daily across DST", anchor: marchLast.AddDate(0, 0, -2), expression: "daily"},
		{name: "weekly across DST", anchor: marchLast.AddDate(0, 0, -7), expression: "weekly"},
		{name: "monthly after a clamped month end", anchor: time.Date(baseYear, time.January, 31, 9, 0, 0, 0, lisbon), expression: "monthly"},
		{name: "month interval", anchor: time.Date(baseYear, time.January, 31, 9, 0, 0, 0, lisbon), expression: "2mo"},
		{name: "quarterly", anchor: time.Date(baseYear, time.January, 15, 9, 0, 0, 0, lisbon), expression: "quarterly"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TZ", "Europe/Lisbon")
			client, env := isolatedClient(t)
			enableRecurrenceLimit(t, env, 4)
			if err := client.Add(context.Background(), domain.NewTask{
				Description: "series comparison",
				Due:         test.anchor.Format("20060102T150405"),
				Recurrence:  test.expression,
			}); err != nil {
				t.Fatal(err)
			}
			tasks, err := client.ExportPending(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(tasks) != 4 {
				t.Fatalf("generated %d tasks, want 4", len(tasks))
			}
			sort.Slice(tasks, func(i, j int) bool { return tasks[i].Due.Before(*tasks[j].Due) })
			same := true
			previous := test.anchor
			for _, generated := range tasks[1:] {
				next, ok := domain.NextInstanceDue(test.anchor, test.expression, previous, test.anchor)
				if !ok {
					t.Fatalf("no occurrence after %v", previous)
				}
				same = same && next.Equal(*generated.Due)
				t.Logf("Taskwarrior %v, backend %v", generated.Due.In(lisbon), next)
				previous = next
			}
			if same != test.same {
				t.Errorf("occurrences agree = %v, want %v", same, test.same)
			}
		})
	}
}

func enableRecurrenceLimit(t *testing.T, env isolatedEnvironment, limit int) {
	t.Helper()
	file, err := os.OpenFile(env.TaskRC, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("recurrence.limit=" + strconv.Itoa(limit) + "\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

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
