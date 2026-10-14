package taskwarrior

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// comparedDates are the expressions domain.ResolveDate supports, in the forms
// Momentum sends to Taskwarrior. The last five sit on Lisbon's DST changes.
var comparedDates = []string{
	"now", "today", "sod", "eod", "yesterday", "tomorrow", "next-week", "later", "someday",
	"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday", "mon", "thurs",
	"january", "february", "march", "april", "may", "june", "july", "august", "september",
	"october", "november", "december", "jan", "sept",
	"sow", "eow", "sopw", "eopw", "sonw", "eonw", "sopww", "eopww", "sonww", "eonww",
	"som", "eom", "sopm", "eopm", "sonm", "eonm", "soq", "eoq", "sopq", "eopq", "sonq", "eonq",
	"soy", "eoy", "sopy", "eopy", "sony", "eony",
	"1st", "2nd", "3rd", "4th", "11th", "12th", "13th", "21st", "22nd", "23rd", "28th",
	"2026-10-20", "2026-10-20 09:30", "2026-10-20T09:30", "2026-10-20T09:30:45", "20261020T093000",
	"20261020T093000Z", "2026-10-20T09:30Z", "2026-10-20T09:30:00+05:00",
	"2026-10-25", "2026-10-25T12:00", "2027-03-28", "2027-03-28 03:30", "2027-03-28T01:30:00Z",
}

// Taskwarrior reads the clock itself, so the zones with a DST change three
// days from now make the relative names cross one whenever the test runs.
func TestIntegrationResolveDateMatchesTaskwarrior(t *testing.T) {
	lisbon, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		t.Fatal(err)
	}
	change := time.Now().UTC().AddDate(0, 0, 3).YearDay() - 1
	opposite := (change + 183) % 365
	ends := fmt.Sprintf("XST-1XDT,%d/2,%d/3", opposite, change)
	starts := fmt.Sprintf("XST-1XDT,%d/2,%d/3", change, opposite)
	for _, zone := range []struct {
		name       string
		tz         string
		loc        *time.Location
		crossesDST bool
	}{
		{"Europe/Lisbon", "Europe/Lisbon", lisbon, false},
		{"summer time ends in three days", ends, posixZone(t, ends), true},
		{"summer time starts in three days", starts, posixZone(t, starts), true},
	} {
		t.Run(zone.name, func(t *testing.T) {
			client, _ := isolatedClient(t)
			t.Setenv("TZ", zone.tz)
			before := time.Now().In(zone.loc)
			for index, expression := range comparedDates {
				if err := client.Add(context.Background(), domain.NewTask{Description: "date " + strconv.Itoa(index), Due: expression}); err != nil {
					t.Fatalf("add due:%s: %v", expression, err)
				}
			}
			after := time.Now().In(zone.loc)
			if before.YearDay() != after.YearDay() {
				t.Skip("the date changed during the comparison")
			}
			tasks, err := client.ExportPending(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(tasks) != len(comparedDates) {
				t.Fatalf("exported %d tasks, want %d", len(tasks), len(comparedDates))
			}
			for _, task := range tasks {
				index, err := strconv.Atoi(strings.TrimPrefix(task.Description, "date "))
				if err != nil || task.Due == nil {
					t.Fatalf("task=%#v", task)
				}
				expression := comparedDates[index]
				want, err := domain.ResolveDate(expression, before)
				if err != nil {
					t.Fatalf("ResolveDate(%q): %v", expression, err)
				}
				switch expression {
				case "now":
					if task.Due.Before(want) || task.Due.After(after) {
						t.Errorf("now: Taskwarrior %v outside [%v, %v]", task.Due.In(zone.loc), want, after)
					}
					continue
				case "next-week":
					// Taskwarrior adds sow+1w+4d as seconds, so across a DST change it
					// lands an hour off Friday midnight; ResolveDate does not.
					sow, _ := domain.ResolveDate("sow", before)
					if zone.crossesDST && offsetOf(want) == offsetOf(before) {
						t.Errorf("next-week %v does not cross the DST change", want)
					}
					want = sow.Add(11 * 24 * time.Hour)
				}
				if !task.Due.Equal(want) {
					t.Errorf("%s: Taskwarrior %v, ResolveDate %v", expression, task.Due.In(zone.loc), want)
				}
			}
		})
	}
}

func offsetOf(value time.Time) int {
	_, offset := value.Zone()
	return offset
}

// posixZone builds the zone that a POSIX TZ rule describes. Taskwarrior reads
// the rule from TZ; Go needs it as the footer of a TZif file.
func posixZone(t *testing.T, rule string) *time.Location {
	t.Helper()
	var data bytes.Buffer
	header := func(counts ...uint32) {
		data.WriteString("TZif2")
		data.Write(make([]byte, 15))
		for _, count := range counts {
			_ = binary.Write(&data, binary.BigEndian, count)
		}
	}
	header(0, 0, 0, 0, 0, 0) // empty version 1 block
	header(0, 0, 0, 0, 1, 4) // one zone type, four bytes of names
	_ = binary.Write(&data, binary.BigEndian, int32(3600))
	data.Write([]byte{0, 0})
	data.WriteString("XST\x00\n" + rule + "\n")
	loc, err := time.LoadLocationFromTZData(rule, data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
