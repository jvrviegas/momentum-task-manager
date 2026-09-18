package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jvrviegas/momentum/internal/domain"
)

func projectKey(code rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: code}) }

func TestQuickAddCatalogMatchesLabelAndSubmitsValue(t *testing.T) {
	q := NewQuickAdd(NewStyles(ResolveTheme("dark", true)), IconsFor("unicode"))
	q.SetSize(100, 30)
	q.SetProjectCatalog(domain.ProjectCatalog{{Name: "Delivery", Value: "work"}, {Name: "Customer", Value: "work.client"}})
	q.OpenQuickAdd("Prepare proposal #customer")
	if len(q.Suggestions) != 1 || q.Suggestions[0].Value != "work.client" {
		t.Fatalf("suggestions=%#v", q.Suggestions)
	}
	if !strings.Contains(q.View(), "Delivery → Customer") {
		t.Fatalf("missing label: %s", q.View())
	}
	q.Update(projectKey(tea.KeyTab))
	if q.Input.Value() != "Prepare proposal #work.client" {
		t.Fatalf("input=%q", q.Input.Value())
	}
	_, cmd := q.Update(projectKey(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("no submit command")
	}
	msg, ok := cmd().(QuickAddSubmitMsg)
	if !ok || msg.Task.Project != "work.client" || msg.Task.Description != "Prepare proposal" {
		t.Fatalf("submit=%#v", msg)
	}
}

func TestQuickAddLongCatalogKeepsSelectedSuggestionVisible(t *testing.T) {
	q := NewQuickAdd(NewStyles(ResolveTheme("dark", true)), IconsFor("unicode"))
	q.SetSize(100, 30)
	var projects domain.ProjectCatalog
	for i := 0; i < 16; i++ {
		projects = append(projects, domain.Project{Name: fmt.Sprintf("Project %02d", i), Value: fmt.Sprintf("project-%02d", i)})
	}
	q.SetProjectCatalog(projects)
	q.OpenQuickAdd("Task #")
	for i := 0; i < 15; i++ {
		q.Update(projectKey(tea.KeyDown))
	}
	if !strings.Contains(q.View(), "Project 15") {
		t.Fatalf("selected project hidden: %s", q.View())
	}
	q.Update(projectKey(tea.KeyTab))
	if q.Input.Value() != "Task #project-15" {
		t.Fatalf("input=%q", q.Input.Value())
	}
}

func TestEditorCatalogMatchesLabelAndAssignsValue(t *testing.T) {
	e := NewEdit(NewStyles(ResolveTheme("dark", true)), IconsFor("unicode"))
	e.SetSize(120, 40)
	e.SetProjectCatalog(domain.ProjectCatalog{{Name: "Customer", Value: "work.client"}})
	e.OpenTask(domain.Task{UUID: "one", Description: "Task", Status: "pending"}, FieldProject)
	e.Inputs[FieldProject].SetValue("customer")
	e.refreshSuggestions()
	if len(e.Suggestions) != 1 || e.Suggestions[0] != "work.client" {
		t.Fatalf("suggestions=%v", e.Suggestions)
	}
	if !strings.Contains(e.View(), "Customer") {
		t.Fatalf("missing label: %s", e.View())
	}
	e.Update(projectKey(tea.KeyEnter))
	if e.Input(FieldProject).Value() != "work.client" {
		t.Fatalf("value=%q", e.Input(FieldProject).Value())
	}
}
