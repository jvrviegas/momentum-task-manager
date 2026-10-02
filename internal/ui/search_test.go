package ui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func searchSpecial(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Mod: mod})
}

func TestFilterTasksCoversDescriptionProjectAndTags(t *testing.T) {
	tasks := []domain.Task{
		{UUID: "description", Description: "Prepare proposal"},
		{UUID: "project", Description: "Review", Project: "work.client"},
		{UUID: "tag", Description: "Call client", Tags: []string{"planning"}},
		{UUID: "none", Description: "Unrelated"},
	}
	for _, query := range []string{"proposal", "client", "plng"} {
		got := FilterTasks(tasks, query)
		if len(got) == 0 {
			t.Errorf("query=%q returned no matches", query)
		}
	}
	if got := FilterTasks(tasks, "zz"); len(got) != 0 {
		t.Fatalf("got=%#v", got)
	}
}

func TestFilterTasksSupportsExactProjectQualifier(t *testing.T) {
	tasks := []domain.Task{
		{UUID: "exact", Description: "Prepare proposal", Project: "Work.Client"},
		{UUID: "prefix", Description: "Prepare proposal", Project: "work.client.archive"},
		{UUID: "description", Description: "work.client proposal", Project: "personal"},
		{UUID: "unassigned", Description: "Prepare proposal"},
	}

	if got := FilterTasks(tasks, "project:work.client"); len(got) != 1 || got[0].UUID != "exact" {
		t.Fatalf("exact project filter=%#v", got)
	}
	if got := FilterTasks(tasks, "project:WORK.CLIENT prepare"); len(got) != 1 || got[0].UUID != "exact" {
		t.Fatalf("combined project filter=%#v", got)
	}
	if got := FilterTasks(tasks, "project:work.client missing"); len(got) != 0 {
		t.Fatalf("combined text should also apply: %#v", got)
	}
	if got := FilterTasks(tasks, "project:none"); len(got) != 1 || got[0].UUID != "unassigned" {
		t.Fatalf("unassigned project filter=%#v", got)
	}
}

func TestFilterTasksPreservesStableScoreTies(t *testing.T) {
	tasks := []domain.Task{{UUID: "first", Description: "alpha"}, {UUID: "second", Description: "alpine"}}
	got := FilterTasks(tasks, "a")
	if len(got) != 2 || got[0].UUID != "first" {
		t.Fatalf("got=%#v", got)
	}
}

func TestFilterTasksEmptyQueryCopiesInput(t *testing.T) {
	tasks := []domain.Task{{UUID: "one"}}
	got := FilterTasks(tasks, " ")
	if len(got) != 1 || &got[0] == &tasks[0] {
		t.Fatalf("got=%#v", got)
	}
}

func TestSearchOpensEditsAndCommitsFilter(t *testing.T) {
	search := NewSearch(Styles{}, Icons{})
	search.SetSize(40, 5)
	search.OpenSearch("")
	search.Input.SetValue("client")
	_, cmd := search.Update(searchSpecial(tea.KeyEnter, 0))
	message, ok := cmd().(SearchCommitMsg)
	if !ok || message.Query != "client" || search.Open || !search.Active || search.Query != "client" {
		t.Fatalf("search=%#v message=%#v", search, message)
	}
}

func TestSearchEscapeClearsFilter(t *testing.T) {
	search := NewSearch(Styles{}, Icons{})
	search.OpenSearch("client")
	search.Update(searchSpecial(tea.KeyEscape, 0))
	if search.Open || search.Active || search.Query != "" || search.Input.Value() != "" {
		t.Fatalf("search=%#v", search)
	}
}

func TestSearchViewIsWidthSafe(t *testing.T) {
	search := NewSearch(NewStyles(ResolveTheme("dark", true)), IconsFor("unicode"))
	search.SetSize(30, 4)
	search.OpenSearch("a very long search value")
	view := search.View()
	if strings.Contains(view, "\n") || len(view) == 0 {
		t.Fatalf("view=%q", view)
	}
}

func TestSearchSummaryShowsMatchCount(t *testing.T) {
	if SearchSummary(10, 3, "work") != "3 of 10 match" || SearchSummary(10, 10, "") != "" {
		t.Fatal("unexpected summary")
	}
}

func TestMatchesTaskUsesOnlyApprovedFields(t *testing.T) {
	task := domain.Task{Description: "Task", RawFields: map[string]json.RawMessage{"secret": json.RawMessage(`"needle"`)}}
	if !MatchesTask(task, "task") || MatchesTask(task, "needle") {
		t.Fatal("search included unsupported raw field")
	}
}
