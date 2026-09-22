package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPlanningDefaultsAndWeekdayCapacity(t *testing.T) {
	defaults := Defaults()
	if defaults.Planning.DailyCapacity != 8*time.Hour || defaults.Planning.Buffer != time.Hour {
		t.Fatalf("planning defaults=%#v", defaults.Planning)
	}
	defaults.Planning.WeekdayCapacity = map[string]string{"monday": "6h"}
	if err := defaults.Validate(); err != nil || defaults.Planning.CapacityFor(time.Monday) != 6*time.Hour || defaults.Planning.CapacityFor(time.Tuesday) != 8*time.Hour {
		t.Fatalf("planning=%#v err=%v", defaults.Planning, err)
	}
}

func TestCalendarConfigRejectsRemoteSourcesAndValidatesLocalSources(t *testing.T) {
	settings := Defaults()
	settings.Calendar.Enabled = true
	settings.Calendar.Paths = []string{"https://example.test/calendar.ics"}
	if err := settings.Validate(); err == nil {
		t.Fatal("remote calendar source should be rejected")
	}
	settings.Calendar.Paths = []string{"calendar.ics"}
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPlanningConfigFromTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := []byte("[planning]\nenabled = true\ndaily_capacity = \"7h\"\nbuffer = \"45m\"\n[planning.weekday_capacity]\nfriday = \"4h\"\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Planning.DailyCapacity != 7*time.Hour || settings.Planning.Buffer != 45*time.Minute || settings.Planning.CapacityFor(time.Friday) != 4*time.Hour {
		t.Fatalf("settings=%#v", settings.Planning)
	}
}
