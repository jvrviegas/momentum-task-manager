package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParseEstimateApprovedFormsAndBoundaries(t *testing.T) {
	cases := []struct {
		input   string
		minutes int
		text    string
	}{
		{input: "15m", minutes: 15, text: "15m"},
		{input: "90m", minutes: 90, text: "1h 30m"},
		{input: "1h", minutes: 60, text: "1h"},
		{input: "1.5h", minutes: 90, text: "1h 30m"},
		{input: "1h30m", minutes: 90, text: "1h 30m"},
		{input: "24h", minutes: MaxEstimateMinutes, text: "24h"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseEstimate(tc.input)
			if err != nil || got.Minutes != tc.minutes || got.String() != tc.text || got.TaskwarriorValue() == "" {
				t.Fatalf("estimate=%#v text=%q err=%v", got, got.String(), err)
			}
		})
	}
}

func TestParseEstimateRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", "0m", "-1h", "1.1m", "1h30.5m", "24h1m", "25h", "half-day", "1h30m2"} {
		if _, err := ParseEstimate(input); err == nil {
			t.Errorf("%q was accepted", input)
		}
	}
	if _, err := ParseEstimate("999999999999999999999999999999999999999h"); err == nil {
		t.Fatal("overflow estimate was accepted")
	}
}

func TestEstimateFormattingAndSuggestions(t *testing.T) {
	cases := map[int]string{1: "1m", 45: "45m", 60: "1h", 90: "1h 30m", 1440: "24h", 1500: "25h"}
	for minutes, want := range cases {
		if got := (Estimate{Minutes: minutes}).String(); got != want {
			t.Errorf("%d minutes=%q want %q", minutes, got, want)
		}
	}
	if got := (Estimate{Minutes: 90}).TaskwarriorValue(); got != "90min" {
		t.Fatalf("Taskwarrior value=%q", got)
	}
	if got := EstimateSuggestions(); !reflect.DeepEqual(got, []string{"15m", "30m", "45m", "1h", "2h", "4h"}) {
		t.Fatalf("suggestions=%v", got)
	}
}

func TestParseTaskwarriorEstimateDistinguishesCalendarMonths(t *testing.T) {
	cases := []struct {
		input   string
		minutes int
	}{
		{input: "PT1H", minutes: 60},
		{input: "PT1H30M", minutes: 90},
		{input: "P1D", minutes: 1440},
		{input: "P1DT1H", minutes: 1500},
		{input: "PT60S", minutes: 1},
		{input: "P1000D", minutes: 1000 * 24 * 60},
	}
	for _, tc := range cases {
		got, err := ParseTaskwarriorEstimate(tc.input)
		if err != nil || got.Minutes != tc.minutes {
			t.Errorf("%s: got=%#v err=%v", tc.input, got, err)
		}
	}
	for _, input := range []string{"P1M", "P1Y", "PT90S", "PT0S", "-PT1H", "P1DT", "PT1H1H", "P1W", "not-a-duration"} {
		if _, err := ParseTaskwarriorEstimate(input); err == nil {
			t.Errorf("%s was accepted", input)
		}
	}
}

func TestParseTaskwarriorEstimateRejectsTruncatedComponents(t *testing.T) {
	for _, input := range []string{"P1", "PT1", "P1D2", "P1T1H"} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseTaskwarriorEstimate(input); err == nil {
				t.Fatalf("%s was accepted", input)
			}
		})
	}
}

func TestTaskUnmarshalKeepsMalformedEstimateRawOnly(t *testing.T) {
	for _, input := range []string{"P1", "PT1", "P1D2", "P1T1H"} {
		var task Task
		data := []byte(`{"uuid":"malformed","estimate":"` + input + `"}`)
		if err := json.Unmarshal(data, &task); err != nil {
			t.Fatalf("%s unmarshal: %v", input, err)
		}
		if task.Estimate != nil || strings.TrimSpace(task.EstimateWarning) == "" {
			t.Fatalf("%s estimate=%#v warning=%q", input, task.Estimate, task.EstimateWarning)
		}
		raw, ok := task.Raw("estimate")
		if !ok || string(raw) != `"`+input+`"` {
			t.Fatalf("%s raw=%s ok=%v", input, raw, ok)
		}
	}
}

func TestTaskUnmarshalDecodesEstimateAndRetainsRawValue(t *testing.T) {
	var task Task
	data := []byte(`{"uuid":"estimated","status":"pending","estimate":"PT1H30M","custom":"kept"}`)
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatal(err)
	}
	if task.Estimate == nil || task.Estimate.Minutes != 90 || task.EstimateWarning != "" {
		t.Fatalf("estimate=%#v warning=%q", task.Estimate, task.EstimateWarning)
	}
	if raw, ok := task.Raw("estimate"); !ok || string(raw) != `"PT1H30M"` {
		t.Fatalf("raw estimate=%s ok=%v", raw, ok)
	}
	if raw, ok := task.Raw("custom"); !ok || string(raw) != `"kept"` {
		t.Fatalf("raw custom=%s ok=%v", raw, ok)
	}
}

func TestTaskUnmarshalKeepsUnsupportedEstimateRawOnly(t *testing.T) {
	cases := []string{
		`{"uuid":"month","estimate":"P1M"}`,
		`{"uuid":"seconds","estimate":"PT1S"}`,
		`{"uuid":"number","estimate":15}`,
	}
	for _, data := range cases {
		var task Task
		if err := json.Unmarshal([]byte(data), &task); err != nil {
			t.Fatal(err)
		}
		if task.Estimate != nil || strings.TrimSpace(task.EstimateWarning) == "" {
			t.Fatalf("data=%s estimate=%#v warning=%q", data, task.Estimate, task.EstimateWarning)
		}
		if _, ok := task.Raw("estimate"); !ok {
			t.Fatalf("data=%s lost raw estimate", data)
		}
	}

	var nullValue, absent Task
	if err := json.Unmarshal([]byte(`{"uuid":"null","estimate":null}`), &nullValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"uuid":"absent"}`), &absent); err != nil {
		t.Fatal(err)
	}
	if nullValue.Estimate != nil || nullValue.EstimateWarning != "" || absent.Estimate != nil || absent.EstimateWarning != "" {
		t.Fatalf("null=%#v absent=%#v", nullValue, absent)
	}
}

func TestEstimateDiffDistinguishesUnchangedSetAndClear(t *testing.T) {
	before := EditSnapshot{Estimate: &Estimate{Minutes: 60}}
	if diff := Diff(before, EditSnapshot{Estimate: &Estimate{Minutes: 60}}); !diff.Empty() {
		t.Fatalf("same estimate changed: %#v", diff)
	}
	set := Diff(before, EditSnapshot{Estimate: &Estimate{Minutes: 45}})
	if set.Estimate.Kind != Set || set.Estimate.Value == nil || set.Estimate.Value.Minutes != 45 {
		t.Fatalf("set diff=%#v", set)
	}
	cleared := Diff(before, EditSnapshot{})
	if cleared.Estimate.Kind != Clear || cleared.Estimate.Empty() || !cleared.Description.Empty() {
		t.Fatalf("clear diff=%#v", cleared)
	}
	if got := Snapshot(Task{Estimate: &Estimate{Minutes: 30}}).Estimate; got == nil || got.Minutes != 30 {
		t.Fatalf("snapshot estimate=%#v", got)
	}
}
