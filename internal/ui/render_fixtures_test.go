package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

// TestComponentRenderFixtures complements focused assertions with a compact,
// ANSI-free visual contract. Update is always opt-in through
// MOMENTUM_UPDATE_GOLDENS=1.
func TestComponentRenderFixtures(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {79, 24}, {49, 18}, {28, 8}} {
		size := size
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			states := []string{"task-comfortable", "task-compact", "details", "help", "quick-capture", "edit", "confirm", "quit", "settings-list", "settings-editor"}
			var builder strings.Builder
			for index, state := range states {
				if index > 0 {
					builder.WriteString("\n\n")
				}
				fmt.Fprintf(&builder, "=== %s ===\n%s", state, fixtureComponentView(size[0], size[1], state))
			}
			assertComponentFixture(t, filepath.Join("testdata", "render", fmt.Sprintf("components-%dx%d.txt", size[0], size[1])), builder.String())
		})
	}
}

func fixtureComponentView(width, height int, state string) string {
	styles := NewStyles(ResolveTheme("dark", true))
	icons := IconsFor("unicode")
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	due := now.Add(7 * time.Hour)
	task := domain.Task{
		UUID: "fixture-task", Description: "Renew the domain registration before the next billing window", Status: "pending",
		Project: "personal.admin", Priority: "H", Due: &due, Tags: []string{"planning", "admin"},
		Annotations: []domain.Annotation{{Description: "Check registrar account and confirm receipt"}}, Dependencies: []string{"fixture-dependency"},
		Recurrence: "yearly", Urgency: 8.25, RawFields: map[string]json.RawMessage{"custom": json.RawMessage(`"fixture value"`)},
	}
	switch state {
	case "task-comfortable":
		return RenderTaskRow(task, TaskRowOptions{Width: max(1, width-4), ShowMetadata: true, Density: DensityComfortable, Selected: true, Now: now, Styles: styles, Icons: icons})
	case "task-compact":
		return RenderTaskRow(task, TaskRowOptions{Width: max(1, width-4), ShowMetadata: false, Density: DensityCompact, Now: now, Styles: styles, Icons: IconsFor("ascii")})
	case "details":
		details := NewDetails(styles)
		details.SetSize(width, height)
		details.OpenTask(task)
		details.Now = now
		return details.View()
	case "help":
		help := NewHelp(styles)
		help.SetSize(width, height)
		help.OpenHelp()
		return help.View()
	case "quick-capture":
		quick := NewQuickAdd(styles, icons)
		quick.Now = now
		quick.SetSize(width, height)
		quick.SetCatalog([]string{"personal.admin", "work"}, []string{"planning", "admin"})
		quick.OpenQuickAdd("Renew #")
		return quick.View()
	case "edit":
		editor := NewEdit(styles, icons)
		editor.Location = time.UTC
		editor.SetSize(width, height)
		editor.SetCatalog([]string{"personal.admin", "work"}, []string{"planning", "admin"})
		editor.OpenTask(task, FieldProject)
		return editor.View()
	case "confirm":
		confirm := NewConfirm(styles)
		confirm.SetSize(width, height)
		confirm.OpenFor("Delete task", "Delete the selected task permanently? This cannot be undone after synchronization.")
		return confirm.View()
	case "quit":
		quit := NewQuit(styles)
		quit.SetSize(width, height)
		quit.OpenQuit()
		return quit.View()
	case "settings-list", "settings-editor":
		settings := NewProjectSettings(styles)
		settings.SetSize(width, height)
		settings.OpenProjects(domain.ProjectCatalog{{Name: "Work", Value: "work"}, {Name: "Client", Value: "work.client"}})
		if state == "settings-editor" {
			settings.OpenEdit(0)
		}
		return settings.View()
	default:
		return ""
	}
}

var componentANSI = regexp.MustCompile("\\x1b\\[[0-?]*[ -/]*[@-~]")

func assertComponentFixture(t *testing.T, path, value string) {
	t.Helper()
	value = sanitizeComponentRender(value)
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
		t.Fatalf("read component fixture %s: %v (set MOMENTUM_UPDATE_GOLDENS=1 intentionally to update)", path, err)
	}
	if got := strings.TrimRight(string(golden), "\n"); got != value {
		t.Fatalf("component fixture %s differs; set MOMENTUM_UPDATE_GOLDENS=1 intentionally to update", path)
	}
}

func sanitizeComponentRender(value string) string {
	value = componentANSI.ReplaceAllString(value, "")
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
