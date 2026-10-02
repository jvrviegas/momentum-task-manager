package quickadd

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

// ErrorKind identifies a user-correctable quick-add parse failure.
type ErrorKind string

const (
	ErrEmptyDescription ErrorKind = "empty_description"
	ErrDuplicateField   ErrorKind = "duplicate_field"
	ErrEmptyValue       ErrorKind = "empty_value"
	ErrInvalidPriority  ErrorKind = "invalid_priority"
)

// ParseError reports the token that made quick-add invalid.
type ParseError struct {
	Kind  ErrorKind
	Field string
	Token string
	Msg   string
}

func (e *ParseError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	if e.Token != "" {
		return fmt.Sprintf("invalid %s token %q", e.Field, e.Token)
	}
	return "invalid quick-add input"
}

// Trigger describes a metadata trigger at a token boundary.
type Trigger rune

const (
	TriggerProject   Trigger = '#'
	TriggerPriority  Trigger = '!'
	TriggerDue       Trigger = '@'
	TriggerScheduled Trigger = '>'
	TriggerTag       Trigger = '+'
)

func (t Trigger) String() string { return string(rune(t)) }

// Parse converts quick-add text into a domain NewTask. It treats metadata only
// when its trigger is the first rune of a whitespace-delimited token.
func Parse(input string) (domain.NewTask, error) {
	tokens := splitTokens(input)
	var result domain.NewTask
	var description []string
	seenScalar := make(map[Trigger]string)
	seenTags := make(map[string]struct{})

	for _, token := range tokens {
		if escaped, ok := unescapeTrigger(token.text); ok {
			description = append(description, escaped)
			continue
		}
		if len([]rune(token.text)) == 0 {
			continue
		}
		first := Trigger([]rune(token.text)[0])
		if !isTrigger(first) {
			description = append(description, token.text)
			continue
		}
		value := string([]rune(token.text)[1:])
		if value == "" {
			return domain.NewTask{}, &ParseError{Kind: ErrEmptyValue, Field: triggerField(first), Token: token.text, Msg: fmt.Sprintf("%s trigger needs a value", first)}
		}
		if first == TriggerPriority {
			priority, err := parsePriority(value)
			if err != nil {
				return domain.NewTask{}, &ParseError{Kind: ErrInvalidPriority, Field: "priority", Token: token.text, Msg: err.Error()}
			}
			if _, exists := seenScalar[first]; exists {
				return domain.NewTask{}, duplicateError(first, token.text)
			}
			seenScalar[first] = value
			result.Priority = priority
			continue
		}
		if first == TriggerTag {
			if _, exists := seenTags[value]; exists {
				continue
			}
			seenTags[value] = struct{}{}
			result.Tags = append(result.Tags, value)
			continue
		}
		if _, exists := seenScalar[first]; exists {
			return domain.NewTask{}, duplicateError(first, token.text)
		}
		seenScalar[first] = value
		switch first {
		case TriggerProject:
			result.Project = value
		case TriggerDue:
			result.Due = value
		case TriggerScheduled:
			result.Scheduled = value
		}
	}

	result.Description = strings.Join(description, " ")
	if strings.TrimSpace(result.Description) == "" {
		return domain.NewTask{}, &ParseError{Kind: ErrEmptyDescription, Msg: "task description cannot be empty after metadata extraction"}
	}
	return result, nil
}

// ParseTask is a descriptive alias for Parse.
func ParseTask(input string) (domain.NewTask, error) { return Parse(input) }

type token struct {
	text       string
	start, end int // rune offsets in the original input
}

func splitTokens(input string) []token {
	runes := []rune(input)
	var tokens []token
	start := -1
	for index, value := range runes {
		if unicode.IsSpace(value) {
			if start >= 0 {
				tokens = append(tokens, token{text: string(runes[start:index]), start: start, end: index})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = index
		}
	}
	if start >= 0 {
		tokens = append(tokens, token{text: string(runes[start:]), start: start, end: len(runes)})
	}
	return tokens
}

func unescapeTrigger(value string) (string, bool) {
	runes := []rune(value)
	if len(runes) < 2 || runes[0] != '\\' || !isTrigger(Trigger(runes[1])) {
		return "", false
	}
	return string(runes[1:]), true
}

func isTrigger(value Trigger) bool {
	switch value {
	case TriggerProject, TriggerPriority, TriggerDue, TriggerScheduled, TriggerTag:
		return true
	default:
		return false
	}
}

func triggerField(trigger Trigger) string {
	switch trigger {
	case TriggerProject:
		return "project"
	case TriggerPriority:
		return "priority"
	case TriggerDue:
		return "due"
	case TriggerScheduled:
		return "scheduled"
	case TriggerTag:
		return "tag"
	default:
		return "metadata"
	}
}

func duplicateError(trigger Trigger, token string) error {
	return &ParseError{
		Kind:  ErrDuplicateField,
		Field: triggerField(trigger),
		Token: token,
		Msg:   fmt.Sprintf("duplicate %s metadata; keep only one %s token", triggerField(trigger), trigger),
	}
}

func parsePriority(value string) (string, error) {
	switch strings.ToLower(value) {
	case "high":
		return "H", nil
	case "medium":
		return "M", nil
	case "low":
		return "L", nil
	case "none":
		return "", nil
	default:
		return "", fmt.Errorf("priority %q must be high, medium, low, or none", value)
	}
}
