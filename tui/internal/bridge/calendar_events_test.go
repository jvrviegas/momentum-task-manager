package bridge

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// macEvent is an event on Wednesday 14 October 2026, in local time, as the
// app reads it from the Mac's Calendar.
func macEvent(from, to int, title string) eventWire {
	at := func(hhmm int) time.Time { return time.Date(2026, 10, 14, hhmm/100, hhmm%100, 0, 0, time.Local) }
	return eventWire{Start: at(from), End: at(to), Title: title}
}

// macEvents are the events of calendarFixture, the same day's meetings the
// file tests read.
func macEvents() []eventWire {
	return []eventWire{
		macEvent(930, 945, "Team stand-up"),
		macEvent(1100, 1200, "Design review"),
		macEvent(1130, 1215, "Lunch with Rui"),
		macEvent(1630, 1730, "Retro"),
		macEvent(700, 800, "Early call"),
	}
}

func planWith(t *testing.T, server *Server, input *calendarInput) planResult {
	t.Helper()
	return result[planResult](t, serve(t, server, request(t, 1, "plan", loadParams{Calendar: input}))[1])
}

func loadWith(t *testing.T, server *Server, input *calendarInput) planSummary {
	t.Helper()
	return result[loadResult](t, serve(t, server, request(t, 1, "load", loadParams{Calendar: input}))[1]).Plan
}

func TestEventsFromTheMacGiveTheSameBusyTimeAsTheSameEventsInAFile(t *testing.T) {
	fromFile := planWith(t, newPlanServer(&planClient{}, calendarSettings(t, testNow.Add(-time.Hour))), nil).Calendar

	// The Mac's Calendar stands in for the files, so [calendar] is off here.
	fromMac := planWith(t, newPlanServer(&planClient{}, config.Defaults()), &calendarInput{Events: macEvents()}).Calendar

	if !reflect.DeepEqual(fromMac, fromFile) {
		t.Fatalf("calendar:\n  mac %+v\n file %+v", fromMac, fromFile)
	}
	if fromMac.BusyMinutes != 120 || !fromMac.Enabled || !fromMac.Available {
		t.Fatalf("calendar = %+v", fromMac)
	}
	if summary := loadWith(t, newPlanServer(&planClient{}, config.Defaults()), &calendarInput{Events: macEvents()}); summary.CalendarMinutes != 120 || !summary.CalendarAvailable {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestTitlesComeBackOnlyWhenTheAppSendsThem(t *testing.T) {
	untitled := macEvents()
	for i := range untitled {
		untitled[i].Title = ""
	}
	server := newPlanServer(&planClient{}, config.Defaults())

	for _, block := range planWith(t, server, &calendarInput{Events: untitled}).Calendar.Blocks {
		if len(block.Titles) != 0 {
			t.Errorf("no title was sent, so none may come back: %+v", block)
		}
	}
	if summary := loadWith(t, server, &calendarInput{Events: untitled}); summary.CalendarMinutes != 120 {
		t.Fatalf("a load needs no titles to count the time: %+v", summary)
	}
	blocks := planWith(t, server, &calendarInput{Events: macEvents()}).Calendar.Blocks
	if len(blocks) != 3 || strings.Join(blocks[1].Titles, "|") != "Design review|Lunch with Rui" {
		t.Fatalf("blocks = %+v", blocks)
	}
}

func TestCalendarsTheAppCouldNotReadMakeTheCalendarUnavailable(t *testing.T) {
	settings := config.Defaults()
	input := &calendarInput{Error: "no access"}
	plan := planWith(t, newPlanServer(&planClient{}, settings), input)
	if busy := plan.Calendar; !busy.Enabled || busy.Available || busy.Error != "no access" || busy.BusyMinutes != 0 || busy.Blocks == nil || len(busy.Blocks) != 0 {
		t.Fatalf("calendar = %+v", busy)
	}
	if plan.CapacityMinutes != 480 {
		t.Fatalf("planning should go on with tasks: %+v", plan)
	}
	if summary := loadWith(t, newPlanServer(&planClient{}, settings), input); summary.CalendarAvailable || summary.CalendarError != "no access" || summary.CalendarMinutes != 0 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestTheMacCalendarNeedsNeitherTheFilesSwitchNorTheirPaths(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.ics")
	settings := config.Defaults()
	settings.Calendar.Enabled = true
	settings.Calendar.Paths = []string{missing}
	server := newPlanServer(&planClient{}, settings)

	if busy := planWith(t, server, nil).Calendar; busy.Available || !strings.Contains(busy.Error, "gone.ics") {
		t.Fatalf("without events the files are read: %+v", busy)
	}
	busy := planWith(t, server, &calendarInput{Events: macEvents()}).Calendar
	if !busy.Available || busy.Error != "" || busy.BusyMinutes != 120 {
		t.Fatalf("with events the files are left alone: %+v", busy)
	}
}

func TestTheHoursAndEventChoicesApplyToTheMacsEventsToo(t *testing.T) {
	events := append(macEvents(),
		eventWire{Start: time.Date(2026, 10, 14, 13, 0, 0, 0, time.Local), End: time.Date(2026, 10, 14, 14, 0, 0, 0, time.Local), Free: true},
		eventWire{Start: time.Date(2026, 10, 14, 0, 0, 0, 0, time.Local), End: time.Date(2026, 10, 14, 23, 59, 59, 0, time.Local), AllDay: true})
	for name, test := range map[string]struct {
		change func(*config.CalendarConfig)
		busy   int
	}{
		"by default":                 {func(*config.CalendarConfig) {}, 120},
		"with free events counted":   {func(c *config.CalendarConfig) { c.IncludeTransparent = true }, 180},
		"with all-day counted":       {func(c *config.CalendarConfig) { c.IncludeAllDay = true }, 480},
		"with shorter hours":         {func(c *config.CalendarConfig) { c.WorkingStart, c.WorkingEnd = "11:00", "12:00" }, 60},
		"a stale limit changes none": {func(c *config.CalendarConfig) { c.StaleAfter = time.Nanosecond }, 120},
	} {
		settings := config.Defaults()
		test.change(&settings.Calendar)
		busy := planWith(t, newPlanServer(&planClient{}, settings), &calendarInput{Events: events}).Calendar
		if busy.BusyMinutes != test.busy || busy.Stale {
			t.Errorf("%s: busy=%d stale=%v, want %d and never stale", name, busy.BusyMinutes, busy.Stale, test.busy)
		}
	}
}
