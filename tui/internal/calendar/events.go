package calendar

import "time"

// FromEvents is Load for events an app has already read, such as the ones in
// the Mac's Calendar. They are narrowed to the day as a file's events are.
// There is no file, so nothing is ever stale.
func FromEvents(events []Event, options Options) LoadResult {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	return LoadResult{
		LoadedAt: now,
		Events:   relevantEvents(events, now, options.IncludeTransparent, options.IncludeAllDay),
	}
}
