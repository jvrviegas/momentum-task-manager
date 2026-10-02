package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
	"github.com/jvrviegas/momentum-task-manager/internal/ui"
)

// TestRenderFixtures is the small deterministic visual contract for the app
// compositor. Set MOMENTUM_UPDATE_GOLDENS=1 deliberately when reviewing an
// intentional presentation change; ordinary runs never rewrite fixtures.
func TestRenderFixtures(t *testing.T) {
	sizes := [][2]int{{120, 30}, {79, 24}, {49, 18}, {28, 8}}
	for _, size := range sizes {
		size := size
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			cases := []struct {
				name string
				view func() string
			}{
				{"today", func() string { return fixtureAppView(size[0], size[1], "today") }},
				{"inbox", func() string { return fixtureAppView(size[0], size[1], "inbox") }},
				{"empty", func() string { return fixtureAppView(size[0], size[1], "empty") }},
				{"loading", func() string { return fixtureAppView(size[0], size[1], "loading") }},
				{"error", func() string { return fixtureAppView(size[0], size[1], "error") }},
				{"search", func() string { return fixtureAppView(size[0], size[1], "search") }},
				{"settings-list", func() string { return fixtureAppView(size[0], size[1], "settings-list") }},
				{"settings-editor", func() string { return fixtureAppView(size[0], size[1], "settings-editor") }},
			}
			var builder strings.Builder
			for index, fixture := range cases {
				if index > 0 {
					builder.WriteString("\n\n")
				}
				rendered := fixture.view()
				assertBoundedRender(t, rendered, size[0], size[1], fixture.name)
				fmt.Fprintf(&builder, "=== %s ===\n%s", fixture.name, rendered)
			}
			assertRenderFixture(t, filepath.Join("testdata", "render", fmt.Sprintf("app-%dx%d.txt", size[0], size[1])), builder.String())
		})
	}
}

func fixtureAppView(width, height int, state string) string {
	model := readyCompositionModel(width, height)
	model.QuickAdd.Now = fixtureNow()
	switch state {
	case "today":
		model.ActiveView = ViewToday
	case "inbox":
		model.ActiveView = ViewInbox
	case "empty":
		model.Tasks = nil
		model.Views = domain.Views{}
		model.ActiveView = ViewInbox
	case "loading":
		model.Tasks = nil
		model.Views = domain.Views{}
		model.ActiveView = ViewToday
		model.Mode = ModeLoading
	case "error":
		model.Tasks = nil
		model.Views = domain.Views{}
		model.ActiveView = ViewToday
		model.Err = errors.New("fixture Taskwarrior unavailable")
		model.Mode = ModeError
	case "search":
		model.ActiveView = ViewToday
		model.Search.Active = true
		model.Search.Query = "scheduled"
	case "details":
		model.ActiveView = ViewToday
		model.OpenDetails()
	case "help":
		model.OpenHelp()
	case "quick-capture":
		model.OpenQuickAdd()
		model.QuickAdd.Input.SetValue("Prepare proposal #work")
		model.QuickAdd.Input.CursorEnd()
		model.QuickAdd.SetCatalog([]string{"work", "personal"}, []string{"planning"})
	case "edit":
		model.ActiveView = ViewToday
		model.OpenEditor(ui.FieldDescription)
	case "settings-list", "settings-editor":
		model.Config.Projects = domain.ProjectCatalog{
			{Name: "Work", Value: "work"},
			{Name: "Client", Value: "work.client"},
		}
		model.SwitchView(ViewSettings)
		if state == "settings-editor" {
			model.ProjectSettings.OpenEdit(0)
		}
	}
	return model.View().Content
}

func fixtureNow() (now time.Time) {
	return time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
}

var ansiEscape = regexp.MustCompile("\\x1b\\[[0-?]*[ -/]*[@-~]")

func assertBoundedRender(t *testing.T, value string, width, height int, state string) {
	t.Helper()
	plain := sanitizeRender(value)
	lines := strings.Split(plain, "\n")
	if len(lines) > height {
		t.Fatalf("state=%s size=%dx%d lines=%d", state, width, height, len(lines))
	}
	for _, line := range lines {
		if lipgloss.Width(line) > width {
			t.Fatalf("state=%s size=%dx%d width=%d line=%q", state, width, height, lipgloss.Width(line), line)
		}
	}
}

func sanitizeRender(value string) string {
	value = ansiEscape.ReplaceAllString(value, "")
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func assertRenderFixture(t *testing.T, path, value string) {
	t.Helper()
	value = sanitizeRender(value)
	if os.Getenv("MOMENTUM_UPDATE_GOLDENS") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read render fixture %s: %v (set MOMENTUM_UPDATE_GOLDENS=1 intentionally to update)", path, err)
	}
	if got := strings.TrimRight(string(golden), "\n"); got != value {
		t.Fatalf("render fixture %s differs; set MOMENTUM_UPDATE_GOLDENS=1 intentionally to update", path)
	}
}
