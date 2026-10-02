package ui

import (
	"testing"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func TestPriorityNoneClearsTheTaskPriority(t *testing.T) {
	e := testEdit()
	e.Inputs[FieldPriority].SetValue("none")
	message := e.submit()().(EditSubmitMsg)
	if message.After.Priority != "" || message.Diff.Priority.Kind != domain.Clear {
		t.Fatalf("message=%#v", message)
	}
}

func TestPriorityTypingAcceptsFriendlyNamesAndNormalizes(t *testing.T) {
	e := testEdit()
	e.Inputs[FieldPriority].SetValue("medium")
	message := e.submit()().(EditSubmitMsg)
	if message.After.Priority != "M" {
		t.Fatalf("priority=%q", message.After.Priority)
	}
}
