package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestEditRejectsInvalidPriority(t *testing.T) {
	e := testEdit()
	e.Inputs[FieldPriority].SetValue("urgent")
	_, cmd := e.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
	message, ok := cmd().(EditErrorMsg)
	if !ok || message.Err == nil || e.Open == false {
		t.Fatalf("message=%#v editor=%#v", message, e)
	}
	e.ApplyMessage(message)
	if e.Err == nil {
		t.Fatal("validation error was not retained")
	}
}
