package bridge

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/taskwarrior"
)

// isolatedServer runs the bridge against a temporary Taskwarrior database. It
// rejects the user's real ~/.taskrc and ~/.task before setting TASKRC/TASKDATA;
// tests using it must not run in parallel.
func isolatedServer(t *testing.T) *Server {
	t.Helper()
	if _, err := exec.LookPath("task"); err != nil {
		t.Skip("Taskwarrior 3.x binary unavailable; isolated integration skipped")
	}
	root := t.TempDir()
	taskrc, data := filepath.Join(root, "taskrc"), filepath.Join(root, "data")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{taskrc, data} {
		if filepath.Clean(path) == filepath.Join(home, ".taskrc") || filepath.Clean(path) == filepath.Join(home, ".task") {
			t.Fatalf("integration guard rejected production Taskwarrior path %q", path)
		}
	}
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	rc := "data.location=" + data + "\nconfirmation=no\nhooks=off\nverbose=no\nuda.estimate.type=duration\nuda.estimate.label=Estimate\n"
	if err := os.WriteFile(taskrc, []byte(rc), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TASKRC", taskrc)
	t.Setenv("TASKDATA", data)
	return NewServer(taskwarrior.NewClient("task"), config.Defaults(), filepath.Join(root, "config.toml"))
}

func call[T any](t *testing.T, server *Server, op string, params any) T {
	t.Helper()
	responses := serve(t, server, request(t, 1, op, params))
	if responses[1].Error != nil {
		t.Fatalf("%s: %s", op, responses[1].Error.Message)
	}
	var result T
	if err := json.Unmarshal(responses[1].Result, &result); err != nil {
		t.Fatalf("%s: decode %s: %v", op, responses[1].Result, err)
	}
	return result
}

func findTask(t *testing.T, result loadResult, description string) taskWire {
	t.Helper()
	for _, task := range result.Tasks {
		if task.Description == description {
			return task
		}
	}
	t.Fatalf("task %q not found in %d tasks", description, len(result.Tasks))
	return taskWire{}
}

func TestIntegrationCaptureEditCompleteAndUndo(t *testing.T) {
	server := isolatedServer(t)
	status := call[statusResult](t, server, "status", nil)
	if status.EstimateUDA != string(taskwarrior.EstimateUDAConfigured) || status.SyncConfigured {
		t.Fatalf("status = %+v", status)
	}

	if added := call[captureResult](t, server, "capture", interpretParams{Input: "Send invoice #work !high @today ~30m"}); !added.Added {
		t.Fatalf("capture = %+v", added)
	}
	loaded := call[loadResult](t, server, "load", nil)
	invoice := findTask(t, loaded, "Send invoice")
	if invoice.TodayGroup != "Due Today" || invoice.Project != "work" || invoice.Priority != "H" || invoice.EstimateMinutes != 30 {
		t.Fatalf("invoice = %+v", invoice)
	}

	// Dates go back to Taskwarrior in the editor's local "YYYY-MM-DD HH:MM" form.
	tomorrow := time.Now().AddDate(0, 0, 1)
	after := invoice.Edit
	after.Due = tomorrow.Format("2006-01-02") + " 09:30"
	after.Estimate = "1h"
	if result := call[modifyResult](t, server, "modify", modifyParams{UUID: invoice.UUID, Before: invoice.Edit, After: after}); !result.Changed {
		t.Fatal("modify reported no change")
	}
	invoice = findTask(t, call[loadResult](t, server, "load", nil), "Send invoice")
	if invoice.Edit.Due != after.Due || invoice.EstimateMinutes != 60 || invoice.TodayGroup != "" {
		t.Fatalf("edited invoice = %+v", invoice)
	}

	review := call[captureResult](t, server, "capture", interpretParams{Input: "Call at 3pm"})
	if review.Added || review.Review == nil {
		t.Fatalf("bare time should need review: %+v", review)
	}
	values := append([]string(nil), review.Review.ReviewValues...)
	values[0], values[1] = "Call the bank", "tomorrow"
	resolved := reviewParams{Token: review.Review.Token, Values: values}
	resolved.Touched[0], resolved.Touched[1] = true, true
	call[struct{}](t, server, "captureReview", resolved)
	if call := findTask(t, call[loadResult](t, server, "load", nil), "Call the bank"); call.Due == "" {
		t.Fatalf("reviewed task = %+v", call)
	}

	call[struct{}](t, server, "complete", uuidParams{UUID: invoice.UUID})
	loaded = call[loadResult](t, server, "load", nil)
	if len(loaded.Completed) != 1 || loaded.Completed[0].Title != "Today" || loaded.Completed[0].UUIDs[0] != invoice.UUID {
		t.Fatalf("completed = %+v", loaded.Completed)
	}
	call[struct{}](t, server, "undo", nil)
	if invoice = findTask(t, call[loadResult](t, server, "load", nil), "Send invoice"); invoice.Status != "pending" {
		t.Fatalf("undo left invoice %q", invoice.Status)
	}
}
