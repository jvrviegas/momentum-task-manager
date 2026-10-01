package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
)

func TestWidePlusShowsReadOnlyDetailsPane(t *testing.T) {
	model := readyCompositionModel(150, 30)
	content := sanitizeRender(model.View().Content)
	lines := strings.Split(content, "\n")
	if !strings.Contains(lines[0], "│ Details") || !strings.Contains(lines[0], "enter expands") || !strings.Contains(content, "Overdue task") {
		t.Fatalf("wide+ content=%s", content)
	}
	// The pane divider sits at column W − 44 on every rail row.
	for y := 0; y < 27; y++ {
		if r := []rune(lines[y]); len(r) <= 106 || r[106] != '│' {
			t.Fatalf("row %d has no pane divider: %q", y, lines[y])
		}
	}
	before := model.Selected[ViewToday]
	model.Update(tea.MouseClickMsg{X: 120, Y: 4, Button: tea.MouseLeft})
	if model.Selected[ViewToday] != before {
		t.Fatalf("pane click changed selection to %q", model.Selected[ViewToday])
	}
}

func TestOverflowingListReportsTasksBelow(t *testing.T) {
	model := readyCompositionModel(28, 8)
	content := sanitizeRender(model.View().Content)
	if !strings.Contains(content, "↓ 1 more") {
		t.Fatalf("content=%s", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if lipgloss.Width(line) > 28 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func TestLoadErrorShowsStderrBlockAndNextStep(t *testing.T) {
	model := readyCompositionModel(120, 30)
	model.Tasks, model.Views = nil, domain.Views{}
	model.Mode = ModeError
	model.Err = &taskwarrior.CommandError{Kind: "export", ExitCode: 2, Stderr: "Could not open ~/.task-work/taskchampion.sqlite3"}
	content := sanitizeRender(model.View().Content)
	for _, want := range []string{"! Could not load tasks", "task export exited with status 2. Your data has not been changed.", " stderr", "Could not open ~/.task-work", "press r to retry", "r  retry"} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %q in %s", want, content)
		}
	}
	if strings.Contains(content, "Inbox            0") {
		t.Fatalf("counts should be omitted when unknown: %s", content)
	}
}

func TestMutationStatusNamesTheTask(t *testing.T) {
	model := readyCompositionModel(120, 30)
	model.PendingMutation = &MutationRequest{Kind: MutationComplete, UUID: "due"}
	model.MutationRunning = true
	model.applyMutation(MutationMsg{Kind: MutationComplete, UUID: "due"})
	if model.Status != `Completed "Due task"` {
		t.Fatalf("status=%q", model.Status)
	}
	model.PendingMutation = &MutationRequest{Kind: MutationAdd, Input: domain.NewTask{Description: "Book flights"}}
	model.applyMutation(MutationMsg{Kind: MutationAdd})
	if model.Status != `Added "Book flights"` {
		t.Fatalf("status=%q", model.Status)
	}
}

func TestCompletedFooterOmitsCapture(t *testing.T) {
	model := readyCompositionModel(120, 30)
	model.ActiveView = ViewCompleted
	footer := strings.Split(sanitizeRender(model.View().Content), "\n")[28]
	if strings.Contains(footer, "capture") || !strings.Contains(footer, "/  search") || !strings.Contains(footer, "1-4  views") {
		t.Fatalf("footer=%q", footer)
	}
}
