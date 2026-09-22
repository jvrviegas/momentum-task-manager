package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func plannerKey(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0]})
}

func TestPlannerTogglesCandidatesButNotFixedObligations(t *testing.T) {
	planner := NewPlanner(NewStyles(ResolveTheme("dark", true)), IconsFor("unicode"))
	planner.SetSize(80, 24)
	planner.OpenPlan(time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC), []PlannerItem{
		{UUID: "fixed", Description: "deadline", Group: PlannerObligation, Fixed: true, Selected: true},
		{UUID: "one", Description: "one", Group: PlannerCandidate},
	}, PlannerSummary{ConfiguredMinutes: 480, AvailableMinutes: 480})
	planner.Update(plannerKey(" "))
	if planner.Items[0].Selected != true || planner.Items[1].Selected != true {
		t.Fatalf("items=%#v", planner.Items)
	}
	planner.Update(plannerKey(" "))
	if planner.Items[1].Selected {
		t.Fatal("candidate did not toggle off")
	}
}

func TestPlannerCommitReturnsSelectedUUIDsAndWarnsOverCapacity(t *testing.T) {
	planner := NewPlanner(NewStyles(ResolveTheme("dark", true)), IconsFor("unicode"))
	planner.SetSize(80, 24)
	planner.OpenPlan(time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC), []PlannerItem{{UUID: "one", Description: "one", Group: PlannerCandidate, Selected: true, Estimate: "2h"}}, PlannerSummary{ConfiguredMinutes: 60, AvailableMinutes: 60, SelectedMinutes: 120, CalendarAvailable: true, MeetingCount: 1, CalendarEvents: []PlannerEvent{{Summary: "Team sync", Start: time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC), End: time.Date(2026, 9, 21, 11, 0, 0, 0, time.UTC)}}})
	view := planner.View()
	if !strings.Contains(view, "Warning") || !strings.Contains(view, "over by") || !strings.Contains(view, "Team sync") || !strings.Contains(view, "10:00") {
		t.Fatalf("view=%q", view)
	}
	cmd := planner.Update(plannerKey("enter"))
	if cmd == nil {
		t.Fatal("commit command missing")
	}
	message, ok := cmd().(PlannerSubmitMsg)
	if !ok || len(message.UUIDs) != 1 || message.UUIDs[0] != "one" {
		t.Fatalf("message=%#v", message)
	}
}
