package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
	"github.com/jvrviegas/momentum-task-manager/internal/ui"
)

func readyCompositionModel(width, height int) *Model {
	model := testModel(&fakeClient{})
	model.Width, model.Height = width, height
	model.Tasks = []domain.Task{
		{UUID: "inbox", Description: "Inbox task", Status: "pending", Project: "work", Urgency: 4},
		{UUID: "overdue", Description: "Overdue task", Status: "pending", Urgency: 10, Due: timePtr(time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC))},
		{UUID: "due", Description: "Due task", Status: "pending", Urgency: 8, Due: timePtr(time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC))},
		{UUID: "scheduled", Description: "Scheduled task", Status: "pending", Urgency: 2, Scheduled: timePtr(time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC))},
	}
	model.Views = domain.BuildViews(model.Tasks, model.now())
	model.ActiveView = ViewToday
	model.Mode = ModeReady
	model.Selected[ViewToday] = "overdue"
	model.Selections[ViewToday] = 0
	return model
}

func timePtr(value time.Time) *time.Time { return &value }

func TestWideViewComposesSidebarAndRows(t *testing.T) {
	model := readyCompositionModel(120, 30)
	view := model.View()
	if !strings.Contains(view.Content, "Inbox") || !strings.Contains(view.Content, "Today") || !strings.Contains(view.Content, "Overdue") || !strings.Contains(view.Content, "Due Today") {
		t.Fatalf("view=%q", view.Content)
	}
	if !view.AltScreen || view.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("view options=%#v", view)
	}
}

func TestCompactViewUsesTabs(t *testing.T) {
	model := readyCompositionModel(79, 24)
	content := model.View().Content
	if !strings.Contains(content, "Inbox") || !strings.Contains(content, "Today") {
		t.Fatalf("content=%q", content)
	}
	// The compact composition does not allocate a standalone 24-column rail.
	if strings.Count(content, "Inbox") < 1 {
		t.Fatal("compact navigation missing")
	}
}

func TestNarrowViewHidesRowMetadata(t *testing.T) {
	model := readyCompositionModel(49, 20)
	content := model.View().Content
	if strings.Contains(content, "#work") || strings.Contains(content, "Today 17:00") {
		t.Fatalf("metadata leaked: %q", content)
	}
	if lipgloss.Width(strings.Split(content, "\n")[0]) > 49 {
		t.Fatal("header exceeded width")
	}
}

func TestMinimumViewShowsWarning(t *testing.T) {
	model := readyCompositionModel(20, 5)
	content := sanitizeRender(model.View().Content)
	if !strings.Contains(content, "Terminal too small") || !strings.Contains(content, "20 x 5") || !strings.Contains(content, "28 x 8") {
		t.Fatalf("content=%q", content)
	}
}

func TestEmptyInboxAndTodayViewsRenderApprovedCopy(t *testing.T) {
	model := readyCompositionModel(100, 20)
	model.Tasks = nil
	model.Views = domain.Views{}
	model.ActiveView = ViewInbox
	if content := model.View().Content; !strings.Contains(content, ui.InboxEmptyTitle) {
		t.Fatalf("inbox content=%q", content)
	}
	model.ActiveView = ViewToday
	if content := model.View().Content; !strings.Contains(content, ui.TodayEmptyTitle) {
		t.Fatalf("today content=%q", content)
	}
}

func TestTodayCompositionKeepsSectionOrderAndNoDuplicate(t *testing.T) {
	model := readyCompositionModel(100, 30)
	content := model.View().Content
	overdue := strings.Index(content, "Overdue")
	due := strings.Index(content, "Due Today")
	scheduled := strings.Index(content, "Scheduled Today")
	if overdue < 0 || due < 0 || scheduled < 0 || !(overdue < due && due < scheduled) {
		t.Fatalf("section order in=%q", content)
	}
	if strings.Count(content, "Due task") != 1 {
		t.Fatalf("duplicate task in=%q", content)
	}
}

func TestQuickAddOverlayIsCenteredWithCommandHints(t *testing.T) {
	model := readyCompositionModel(100, 30)
	model.OpenQuickAdd()
	content := sanitizeRender(model.View().Content)
	for _, want := range []string{"Quick capture", "TRIGGERS", "# project", "! priority", "@ due", "> scheduled", "+ tag"} {
		if !strings.Contains(content, want) {
			t.Fatalf("quick capture missing %q: %q", want, content)
		}
	}
	// Quick capture sits at row 5 so the list stays visible below it, and is
	// centred horizontally over the dimmed view.
	lines := strings.Split(content, "\n")
	if len(lines) < 6 || !strings.Contains(lines[5], "Quick capture") {
		t.Fatalf("quick capture is not at row 5: %q", content)
	}
	left := strings.Index(lines[5], "╭")
	if left <= 0 || lipgloss.Width(lines[5][:left]) != (100-78)/2 {
		t.Fatalf("quick capture is not horizontally centered: %q", lines[5])
	}
}

func TestEditDetailsAndHelpOverlaysCompose(t *testing.T) {
	model := readyCompositionModel(100, 30)
	model.OpenDetails()
	if content := model.View().Content; !strings.Contains(content, "Task details") {
		t.Fatalf("details=%q", content)
	}
	model.Overlay = OverlayHelp
	model.Help.OpenHelp()
	model.Help.SetSize(100, 30)
	if content := sanitizeRender(model.View().Content); !strings.Contains(content, "╭─ Keys") || !strings.Contains(content, "NAVIGATE") {
		t.Fatalf("help=%q", content)
	}
}

func TestSearchFiltersCurrentViewAndReportsMatches(t *testing.T) {
	model := readyCompositionModel(100, 30)
	model.Search.Active = true
	model.Search.Query = "scheduled"
	content := sanitizeRender(model.View().Content)
	if !strings.Contains(content, "Scheduled task") || strings.Contains(content, "Overdue task") || !strings.Contains(content, "1 of 3 match") {
		t.Fatalf("search content=%q", content)
	}
}

func TestMouseClickSelectsTasksAndSidebarViews(t *testing.T) {
	model := readyCompositionModel(120, 30)
	model.ActiveView = ViewInbox
	model.Views.Today = nil
	model.Views.Inbox = model.Tasks
	// Rail rows: name, blank, VIEWS, Inbox, Today.
	model.Update(tea.MouseClickMsg{X: 5, Y: 4, Button: tea.MouseLeft})
	if model.ActiveView != ViewToday {
		t.Fatalf("sidebar click active=%s", model.ActiveView)
	}
	model.ActiveView = ViewInbox
	// Body row 3 is the #work heading; row 4 is its first task.
	model.Update(tea.MouseClickMsg{X: 40, Y: 4, Button: tea.MouseLeft})
	if model.Selected[ViewInbox] == "" {
		t.Fatalf("task click did not select: %#v", model.Selected)
	}
}

func TestMouseWheelMovesSelection(t *testing.T) {
	model := readyCompositionModel(120, 30)
	model.ActiveView = ViewInbox
	model.Views.Today = nil
	model.Views.Inbox = model.Tasks
	model.Selections[ViewInbox] = 0
	model.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if model.Selections[ViewInbox] != 1 {
		t.Fatalf("selection=%d", model.Selections[ViewInbox])
	}
}

func TestCreateKeyOpensQuickAdd(t *testing.T) {
	model := readyCompositionModel(100, 30)
	model.Update(key("c"))
	if model.Overlay != OverlayQuickAdd || !model.QuickAdd.Open {
		t.Fatalf("quick add not open: overlay=%s open=%v", model.Overlay, model.QuickAdd.Open)
	}
}

func TestGlobalFocusAndSearchKeys(t *testing.T) {
	model := readyCompositionModel(100, 30)
	model.Update(key("h"))
	if model.Focus != FocusSidebar {
		t.Fatal("h did not focus sidebar")
	}
	model.Update(key("l"))
	if model.Focus != FocusList {
		t.Fatal("l did not focus list")
	}
	model.Update(key("/"))
	if model.Overlay != OverlaySearch || !model.Search.Open {
		t.Fatalf("search=%#v", model.Search)
	}
}

func TestSearchEnterKeepsFilterAndEscapeClearsIt(t *testing.T) {
	model := readyCompositionModel(100, 30)
	model.Update(key("/"))
	model.Search.Input.SetValue("scheduled")
	model.Search.Input.CursorEnd()
	_, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("enter did not emit search message")
	}
	message := cmd()
	model.Update(message)
	if !model.Search.Active || model.Search.Query != "scheduled" || model.Overlay != OverlayNone {
		t.Fatalf("search=%#v overlay=%s", model.Search, model.Overlay)
	}
	model.Update(key("esc"))
	if model.Search.Active {
		t.Fatal("escape did not clear filter")
	}
}

func TestOverlayBlocksGlobalMutationKeyInComposition(t *testing.T) {
	model := readyCompositionModel(100, 30)
	model.OpenHelp()
	model.Update(key(" "))
	if model.MutationRunning {
		t.Fatal("help overlay allowed mutation")
	}
}
