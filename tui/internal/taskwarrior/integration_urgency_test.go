package taskwarrior

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// Taskwarrior reads the clock itself, so domain.SetUrgency is checked at
// every second the export may have used.
func TestIntegrationSetUrgencyMatchesTaskwarrior(t *testing.T) {
	newIsolatedEnvironment(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Second)
	date := func(offset time.Duration) string { return base.Add(offset).UTC().Format("20060102T150405Z") }
	notes := func(count int) []map[string]string {
		annotations := make([]map[string]string, count)
		for index := range annotations {
			annotations[index] = map[string]string{"entry": date(-time.Duration(index+1) * time.Minute), "description": "note"}
		}
		return annotations
	}
	day, hour := 24*time.Hour, time.Hour
	fixtures := []map[string]any{
		{"description": "plain"},
		{"description": "project", "project": "work.client"},
		{"description": "priority H", "priority": "H"},
		{"description": "priority M", "priority": "M"},
		{"description": "priority L", "priority": "L"},
		{"description": "one tag", "tags": []string{"a"}},
		{"description": "two tags", "tags": []string{"a", "b"}},
		{"description": "three tags", "tags": []string{"a", "b", "c"}},
		{"description": "next tag", "tags": []string{"next"}},
		{"description": "one annotation", "annotations": notes(1)},
		{"description": "two annotations", "annotations": notes(2)},
		{"description": "four annotations", "annotations": notes(4)},
		{"description": "due in 20 days", "due": date(20 * day)},
		{"description": "due in 3 days 5 hours", "due": date(3*day + 5*hour)},
		{"description": "overdue 2 days 7 hours", "due": date(-(2*day + 7*hour))},
		{"description": "overdue 30 days", "due": date(-30 * day)},
		{"description": "scheduled an hour ago", "scheduled": date(-hour)},
		{"description": "scheduled now", "scheduled": date(0)},
		{"description": "scheduled in an hour", "scheduled": date(hour)},
		{"description": "waiting", "wait": date(2 * day)},
		{"description": "wait passed", "wait": date(-day)},
		{"description": "active", "start": date(-hour)},
		{"description": "blocked", "depends": []string{"middle"}},
		{"description": "middle", "depends": []string{"blocker"}},
		{"description": "blocker"},
		{"description": "blocked by waiting", "depends": []string{"waiting"}},
		{"description": "blocked by done", "depends": []string{"done"}},
		{"description": "done", "status": "completed", "end": date(-hour)},
		{"description": "blocked by deleted", "depends": []string{"deleted"}},
		{"description": "deleted", "status": "deleted", "end": date(-hour)},
		{"description": "10 days old", "entry": date(-(10*day + hour))},
		{"description": "400 days old", "entry": date(-400 * day)},
		{"description": "entry in 3 days", "entry": date(3 * day)},
		{
			"description": "everything", "project": "home", "priority": "H", "tags": []string{"next", "x"}, "annotations": notes(2),
			"due": date(-day), "scheduled": date(-2 * hour), "start": date(-hour), "entry": date(-30 * day),
		},
		{"description": "needs everything", "depends": []string{"everything"}},
	}
	uuids := map[string]string{}
	for index, fixture := range fixtures {
		uuids[fixture["description"].(string)] = fmt.Sprintf("00000000-0000-4000-8000-%012d", index+1)
	}
	for _, fixture := range fixtures {
		fixture["uuid"] = uuids[fixture["description"].(string)]
		if _, ok := fixture["status"]; !ok {
			fixture["status"] = "pending"
		}
		if _, ok := fixture["entry"]; !ok {
			fixture["entry"] = date(0)
		}
		if names, ok := fixture["depends"].([]string); ok {
			dependencies := make([]string, len(names))
			for index, name := range names {
				dependencies[index] = uuids[name]
			}
			fixture["depends"] = dependencies
		}
	}
	data, err := json.Marshal(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fixtures.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, "task", "import", path).CombinedOutput(); err != nil {
		t.Fatalf("task import: %v\n%s", err, output)
	}

	// Every status, so the waiting task is there to block its dependent.
	before := time.Now().Unix()
	output, err := exec.CommandContext(ctx, "task", "export").Output()
	after := time.Now().Unix()
	if err != nil {
		t.Fatalf("task export: %v", err)
	}
	var tasks []domain.Task
	if err := json.Unmarshal(output, &tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != len(fixtures) {
		t.Fatalf("exported %d tasks, want %d", len(tasks), len(fixtures))
	}

	ours := map[string][]float64{}
	for second := before; second <= after; second++ {
		scored := slices.Clone(tasks)
		domain.SetUrgency(scored, time.Unix(second, 0))
		for _, task := range scored {
			ours[task.UUID] = append(ours[task.UUID], task.Urgency)
		}
	}
	for _, task := range tasks {
		if !slices.Contains(ours[task.UUID], task.Urgency) {
			t.Errorf("%s: Taskwarrior %v, SetUrgency %v", task.Description, task.Urgency, ours[task.UUID])
		}
	}
}
