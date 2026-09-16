package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/jvrviegas/momentum/internal/config"
	"github.com/jvrviegas/momentum/internal/domain"
)

func TestConfiguredProjectsAvailableBeforeDiscoveryAndSurviveRefresh(t *testing.T) {
	settings := config.Defaults()
	settings.Projects = domain.ProjectCatalog{{Name: "Work", Value: "work"}, {Name: "Client", Value: "work.client"}}
	client := &fakeClient{}
	model := New(client, settings, ViewInbox)
	assertProjects := func(want []string) {
		t.Helper()
		if !reflect.DeepEqual(model.QuickAdd.Projects, want) || !reflect.DeepEqual(model.Editor.Projects, want) {
			t.Fatalf("quickadd=%v editor=%v want=%v", model.QuickAdd.Projects, model.Editor.Projects, want)
		}
	}
	assertProjects([]string{"work", "work.client"})
	model.Update(ProjectsMsg{Values: []string{"work", "personal", "personal"}})
	assertProjects([]string{"work", "work.client", "personal"})
	model.Update(TagsMsg{Values: []string{"planning"}})
	if model.QuickAdd.ProjectLabels["work.client"] != "Work → Client" || model.Editor.ProjectLabels["work.client"] != "Work → Client" {
		t.Fatal("tag update lost project labels")
	}
	model.Update(ProjectsMsg{Err: context.Canceled})
	assertProjects([]string{"work", "work.client", "personal"})
	model.Update(ProjectsMsg{})
	assertProjects([]string{"work", "work.client"})
	if len(client.mutations) != 0 {
		t.Fatal("catalog updates mutated tasks")
	}
	settings.Projects = nil
	fresh := New(client, settings, ViewInbox)
	if len(fresh.QuickAdd.Projects) != 0 || len(client.mutations) != 0 {
		t.Fatal("removing catalog should only remove suggestions")
	}
}
