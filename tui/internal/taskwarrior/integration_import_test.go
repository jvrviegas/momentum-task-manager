package taskwarrior

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestIntegrationExportAllChangesNothing reads a profile in which an export
// with recurrence on would create a daily series' overdue instances.
func TestIntegrationExportAllChangesNothing(t *testing.T) {
	client, env := isolatedClient(t)
	for _, args := range [][]string{
		{"add", "Daily thing", "due:2026-10-01", "recur:daily", "rc.recurrence=on"},
		{"add", "Plain"},
		{"add", "Finished"},
		{"2", "done"},
		{"add", "Dropped"},
		{"3", "delete"},
	} {
		if output, err := exec.Command("task", args...).CombinedOutput(); err != nil {
			t.Fatalf("task %v: %v\n%s", args, err, output)
		}
	}
	database := filepath.Join(env.DataDir, "taskchampion.sqlite3")
	before, err := os.ReadFile(database)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := client.ExportAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]int{}
	for _, task := range tasks {
		statuses[task.Status]++
	}
	if len(tasks) != 4 || statuses["recurring"] != 1 || statuses["pending"] != 1 || statuses["completed"] != 1 || statuses["deleted"] != 1 {
		t.Errorf("exported %d tasks: %v", len(tasks), statuses)
	}
	after, err := os.ReadFile(database)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("ExportAll changed Taskwarrior's data")
	}
	// The same export with recurrence on writes the instances, so the test
	// would notice an override that stopped working.
	if output, err := exec.Command("task", "rc.recurrence=on", "export").Output(); err != nil || bytes.Count(output, []byte(`"parent"`)) == 0 {
		t.Errorf("an export with recurrence on created no instance: %v", err)
	}
}
