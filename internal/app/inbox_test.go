package app

import (
	"strings"
	"testing"

	"github.com/jvrviegas/momentum/internal/domain"
)

func TestInboxProjectHeadersFilteringAndSelection(t *testing.T) {
	model := readyCompositionModel(120, 30)
	model.ActiveView = ViewInbox
	model.Tasks = []domain.Task{
		{UUID: "a", Description: "First item", Project: "Alpha", Status: "pending"},
		{UUID: "b", Description: "Second item", Project: "Beta", Status: "pending"},
		{UUID: "c", Description: "Unassigned item", Status: "pending"},
	}
	model.Views = domain.BuildViews(model.Tasks, model.now())
	model.Selected[ViewInbox] = "c"
	body := model.renderTaskBody(80, 20)
	previous := -1
	for _, text := range []string{"Alpha", "First item", "Beta", "Second item", "No project", "Unassigned item"} {
		index := strings.Index(body, text)
		if index <= previous {
			t.Fatalf("missing or misordered %q in %q", text, body)
		}
		previous = index
	}
	if body := model.renderTaskBody(80, 2); !strings.Contains(body, "Unassigned item") {
		t.Fatalf("selected task not visible: %q", body)
	}
	model.Search.Active = true
	model.Search.Query = "Second"
	body = sanitizeRender(model.renderTaskBody(80, 20))
	if !strings.Contains(body, "Beta") || !strings.Contains(body, "Second item") || strings.Contains(body, "Alpha") || strings.Contains(body, "No project") {
		t.Fatalf("filtered headers: %q", body)
	}
}
