package app

import (
	"testing"
	"time"
)

func TestUndoWindowOpensForTheDelayAndCloses(t *testing.T) {
	at := modelNow()
	window := UndoWindow{Delay: 15 * time.Second}.Open(at)
	if !window.Available(at) || !window.Available(at.Add(14*time.Second)) || window.Available(at.Add(15*time.Second)) {
		t.Fatalf("window %#v", window)
	}
	if later := window.Open(at.Add(10 * time.Second)); !later.Until.Equal(at.Add(25 * time.Second)) {
		t.Errorf("a later change did not restart the window: %#v", later)
	}
	if closed := window.Close(); closed.Available(at) || closed.Delay != window.Delay {
		t.Errorf("closed %#v", closed)
	}
	if none := (UndoWindow{}).Open(at); none.Available(at) {
		t.Errorf("a zero delay opened the window: %#v", none)
	}
}
