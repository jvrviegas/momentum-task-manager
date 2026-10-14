package quickadd

import (
	"reflect"
	"testing"
	"time"
)

func TestContextAtProjectTokenAndCursor(t *testing.T) {
	input := "prepare #wor now"
	ctx := ContextAt(input, len([]rune("prepare #wor")))
	if !ctx.Active || ctx.Kind != SuggestionProject || ctx.Trigger != TriggerProject || ctx.Prefix != "wor" || ctx.TokenStart != 8 || ctx.TokenEnd != 12 {
		t.Fatalf("unexpected context: %#v", ctx)
	}
}

func TestContextAtDoesNotActivateInsideEmailOrEscapedToken(t *testing.T) {
	for _, input := range []string{"email bob@example.com", `literal \#launch`, "foo #project"} {
		ctx := ContextAt(input, len([]rune(input)))
		if input != "foo #project" && ctx.Active {
			t.Errorf("%q unexpectedly active: %#v", input, ctx)
		}
	}
}

func TestContextAtSupportsCursorInsideToken(t *testing.T) {
	input := "#work.client"
	ctx := ContextAt(input, 4)
	if !ctx.Active || ctx.Prefix != "wor" || ctx.Token != input || ctx.Cursor != 4 {
		t.Fatalf("unexpected context: %#v", ctx)
	}
}

func TestSuggestionsUseFuzzyDeterministicOrdering(t *testing.T) {
	ctx := ContextAt("#wc", 3)
	got := SuggestionsFor(ctx, []string{"work.client", "client", "work-core", "other"}, nil, time.Now())
	if len(got) != 2 {
		t.Fatalf("suggestions=%#v", got)
	}
	if got[0].Value != "work-core" || got[1].Value != "work.client" {
		t.Fatalf("unexpected order: %#v", got)
	}
	if got[0].Text != "#work-core" {
		t.Fatalf("text=%q", got[0].Text)
	}
}

func TestSuggestionsForTagsDeduplicateCandidates(t *testing.T) {
	ctx := ContextAt("+pl", 3)
	got := SuggestionsFor(ctx, nil, []string{"planning", "PLANNING", "plain"}, time.Now())
	if len(got) != 2 || got[0].Value != "plain" || got[1].Value != "planning" {
		t.Fatalf("suggestions=%#v", got)
	}
}

func TestEstimateSuggestionsAreFixed(t *testing.T) {
	ctx := ContextAt("~", 1)
	got := SuggestionsFor(ctx, nil, nil, time.Now())
	values := make([]string, 0, len(got))
	for _, suggestion := range got {
		values = append(values, suggestion.Value)
	}
	if !reflect.DeepEqual(values, []string{"15m", "30m", "45m", "1h", "2h", "4h"}) {
		t.Fatalf("values=%#v", values)
	}
	if got[0].Kind != SuggestionEstimate || got[0].Text != "~15m" {
		t.Fatalf("suggestions=%#v", got)
	}
}

func TestPrioritySuggestionsAreFixed(t *testing.T) {
	ctx := ContextAt("!", 1)
	got := SuggestionsFor(ctx, nil, nil, time.Now())
	values := make([]string, 0, len(got))
	for _, suggestion := range got {
		values = append(values, suggestion.Value)
	}
	if !reflect.DeepEqual(values, []string{"high", "medium", "low", "none"}) {
		t.Fatalf("values=%#v", values)
	}
}

func TestDateSuggestionsIncludeApprovedGuidance(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	ctx := ContextAt("@to", 3)
	got := SuggestionsFor(ctx, nil, nil, now)
	if len(got) == 0 || got[0].Value != "today" {
		t.Fatalf("suggestions=%#v", got)
	}
	all := SuggestionsFor(ContextAt("@", 1), nil, nil, now)
	seen := map[string]bool{}
	for _, suggestion := range all {
		seen[suggestion.Value] = true
	}
	for _, want := range []string{"today", "tomorrow", "next-week", "2026-09-15"} {
		if !seen[want] {
			t.Errorf("missing %q in %#v", want, all)
		}
	}
}

func TestApplySuggestionReplacesWholeToken(t *testing.T) {
	input := "prepare #wor now"
	ctx := ContextAt(input, 12)
	got, cursor := ApplySuggestion(input, ctx, Suggestion{Kind: SuggestionProject, Value: "work.client", Text: "#work.client"})
	if got != "prepare #work.client now" || cursor != len([]rune("prepare #work.client")) {
		t.Fatalf("got %q cursor=%d", got, cursor)
	}
}

func TestApplySuggestionNoopWhenContextInactive(t *testing.T) {
	input := "bob@example.com"
	ctx := ContextAt(input, len([]rune(input)))
	got, cursor := ApplySuggestion(input, ctx, Suggestion{Text: "@today"})
	if got != input || cursor != len([]rune(input)) {
		t.Fatalf("got %q cursor=%d", got, cursor)
	}
}

func TestSuggestionContextAlias(t *testing.T) {
	if got := SuggestionContextAt("+tag", 4); !got.Active || got.Kind != SuggestionTag {
		t.Fatalf("unexpected context: %#v", got)
	}
	if got := SuggestionContextAt("~1h", 3); !got.Active || got.Kind != SuggestionEstimate {
		t.Fatalf("unexpected estimate context: %#v", got)
	}
}
