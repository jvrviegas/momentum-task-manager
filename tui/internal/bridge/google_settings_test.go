package bridge

import (
	"os"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

func TestSettingsShowTheCalendarSource(t *testing.T) {
	for name, test := range map[string]struct{ text, want string }{
		"no [calendar] at all": {backendTable, "files"},
		"no source key":        {backendTable + "[calendar]\nenabled = false\n", "files"},
		"files":                {backendTable + "[calendar]\nsource = \"files\"\n", "files"},
		"google":               {backendTable + "[calendar]\nsource = \"google\"\n", "google"},
	} {
		if got := settingsOf(t, newConfigFile(t, test.text).server()).Calendar.Source; got != test.want {
			t.Errorf("%s: source = %q, want %q", name, got, test.want)
		}
	}
}

func TestSavingTheSourceMakesPlanTodayReadGoogleAtOnce(t *testing.T) {
	file := newConfigFile(t, "# mine\n"+backendTable)
	server := file.server()
	client := &googleClient{events: googleEvents()}
	server.client = client
	if plan := loadOf(t, server).Plan; plan.CalendarAvailable || client.fetches() != 0 {
		t.Fatalf("before choosing Google: %+v, %d fetches", plan, client.fetches())
	}

	google := config.CalendarSourceGoogle
	got := saved(t, server, "saveCalendar", saveCalendarParams{Revision: settingsOf(t, server).Revision, Source: &google})
	if got.Calendar.Source != "google" {
		t.Fatalf("the answer shows %q", got.Calendar.Source)
	}
	if text, _ := os.ReadFile(file.path); !strings.Contains(string(text), `source = "google"`) || !strings.Contains(string(text), "# mine") {
		t.Fatalf("config.toml:\n%s", text)
	}
	if plan := loadOf(t, server).Plan; !plan.CalendarAvailable || plan.CalendarMinutes != 120 || client.fetches() != 1 {
		t.Fatalf("after choosing Google: %+v, %d fetches", plan, client.fetches())
	}

	// And back to files, which are off here: Google is no longer asked.
	files := config.CalendarSourceFiles
	saved(t, server, "saveCalendar", saveCalendarParams{Revision: settingsOf(t, server).Revision, Source: &files})
	if plan := loadOf(t, server).Plan; plan.CalendarAvailable || client.fetches() != 1 {
		t.Fatalf("after choosing files: %+v, %d fetches", plan, client.fetches())
	}
}

func TestChoosingFilesWithTheSwitchOnAndNoneToReadIsRefused(t *testing.T) {
	file := newConfigFile(t, backendTable+"[calendar]\nsource = \"google\"\nenabled = true\n")
	server := file.server()
	before, _ := os.ReadFile(file.path)
	files := config.CalendarSourceFiles
	refusal := refused(t, server, "saveCalendar", saveCalendarParams{Revision: settingsOf(t, server).Revision, Source: &files})
	if refusal.Field != "source" || !strings.Contains(refusal.Message, "Add a calendar file first") {
		t.Errorf("refusal = %+v", refusal)
	}
	if after, _ := os.ReadFile(file.path); string(after) != string(before) {
		t.Errorf("a refused save changed the file:\n%s", after)
	}
}
