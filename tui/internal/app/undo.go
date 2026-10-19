package app

import "time"

// UndoWindow is how long the latest change can be undone. The server keeps
// one step of history per device, and the app offers to undo it for
// mutation_delay after the change (backend plan D7).
type UndoWindow struct {
	Delay time.Duration
	// Until is when the window closes; zero while nothing can be undone.
	Until time.Time
}

// Open starts the window for a change made at at. A zero delay keeps it
// closed.
func (w UndoWindow) Open(at time.Time) UndoWindow {
	w.Until = time.Time{}
	if w.Delay > 0 {
		w.Until = at.Add(w.Delay)
	}
	return w
}

// Close ends the window, as an undo or a project migration does.
func (w UndoWindow) Close() UndoWindow {
	w.Until = time.Time{}
	return w
}

// Available reports whether undo is offered at at.
func (w UndoWindow) Available(at time.Time) bool {
	return w.Until.After(at)
}
