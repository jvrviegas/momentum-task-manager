package quickadd

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jvrviegas/momentum/internal/domain"
)

func TestParseAllMetadataTriggers(t *testing.T) {
	got, err := Parse("Prepare proposal #work.client !high @tomorrow >monday +planning +client")
	if err != nil {
		t.Fatal(err)
	}
	want := domain.NewTask{
		Description: "Prepare proposal",
		Project:     "work.client",
		Priority:    "H",
		Due:         "tomorrow",
		Scheduled:   "monday",
		Tags:        []string{"planning", "client"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParsePreservesPlainTextAndNormalizesWhitespace(t *testing.T) {
	got, err := Parse("  call   bob@example.com about #launch  ")
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "call bob@example.com about" || got.Project != "launch" {
		t.Fatalf("unexpected parsed task: %#v", got)
	}
}

func TestParseTriggerMustBeAtTokenBoundary(t *testing.T) {
	got, err := Parse("email bob@example.com use foo#bar and x+tag")
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "email bob@example.com use foo#bar and x+tag" || got.Project != "" || len(got.Tags) != 0 {
		t.Fatalf("boundary parsing changed text: %#v", got)
	}
}

func TestParseEscapedTriggersBecomeLiteral(t *testing.T) {
	got, err := Parse(`ship \#launch \@today \>monday \+tag \!high`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "ship #launch @today >monday +tag !high" {
		t.Fatalf("got description %q", got.Description)
	}
}

func TestParseDuplicateScalarFieldsReturnTypedErrors(t *testing.T) {
	for _, input := range []string{"one #a #b", "one !high !low", "one @today @tomorrow", "one >today >monday"} {
		_, err := Parse(input)
		var parseErr *ParseError
		if !errors.As(err, &parseErr) || parseErr.Kind != ErrDuplicateField {
			t.Errorf("%q: got %T %v", input, err, err)
		}
	}
}

func TestParseDuplicateTagsAreDeduplicated(t *testing.T) {
	got, err := Parse("one +a +b +a +B")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Tags, []string{"a", "b", "B"}) {
		t.Fatalf("tags=%#v", got.Tags)
	}
}

func TestParsePriorityValues(t *testing.T) {
	cases := map[string]string{"!high": "H", "!medium": "M", "!low": "L", "!none": ""}
	for token, want := range cases {
		got, err := Parse("task " + token)
		if err != nil || got.Priority != want {
			t.Errorf("%s: got %#v err=%v", token, got, err)
		}
	}
}

func TestParseRejectsInvalidPriority(t *testing.T) {
	_, err := Parse("task !urgent")
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Kind != ErrInvalidPriority {
		t.Fatalf("got %T %v", err, err)
	}
}

func TestParseRejectsEmptyMetadataValues(t *testing.T) {
	for _, input := range []string{"task #", "task !", "task @", "task >", "task +"} {
		_, err := Parse(input)
		var parseErr *ParseError
		if !errors.As(err, &parseErr) || parseErr.Kind != ErrEmptyValue {
			t.Errorf("%q: got %T %v", input, err, err)
		}
	}
}

func TestParseRejectsEmptyDescriptionAfterMetadata(t *testing.T) {
	for _, input := range []string{"", "   ", "#work", "!high", "+tag"} {
		_, err := Parse(input)
		var parseErr *ParseError
		if !errors.As(err, &parseErr) || parseErr.Kind != ErrEmptyDescription {
			t.Errorf("%q: got %T %v", input, err, err)
		}
	}
}

func TestParseAllowsUnknownProjectsAndTags(t *testing.T) {
	got, err := Parse("new thing #new.project +newtag")
	if err != nil || got.Project != "new.project" || !reflect.DeepEqual(got.Tags, []string{"newtag"}) {
		t.Fatalf("got %#v err=%v", got, err)
	}
}

func TestParseTaskAlias(t *testing.T) {
	got, err := ParseTask("task")
	if err != nil || got.Description != "task" {
		t.Fatalf("got %#v err=%v", got, err)
	}
}
