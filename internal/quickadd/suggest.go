package quickadd

import (
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

// SuggestionKind identifies the source field for a completion.
type SuggestionKind string

const (
	SuggestionProject    SuggestionKind = "project"
	SuggestionPriority   SuggestionKind = "priority"
	SuggestionDue        SuggestionKind = "due"
	SuggestionScheduled  SuggestionKind = "scheduled"
	SuggestionTag        SuggestionKind = "tag"
	SuggestionEstimate   SuggestionKind = "estimate"
	SuggestionRecurrence SuggestionKind = "recurrence"
)

// SuggestionContext describes the token under a cursor when it is a known
// metadata trigger. Cursor and token offsets are rune offsets.
type SuggestionContext struct {
	Active     bool
	Kind       SuggestionKind
	Trigger    Trigger
	Token      string
	Prefix     string
	Cursor     int
	TokenStart int
	TokenEnd   int
}

// Suggestion is a deterministic completion candidate. Text is the complete
// token to insert, while Value is the unprefixed field value.
type Suggestion struct {
	Kind  SuggestionKind
	Value string
	Text  string
	Label string // Optional display label; never inserted into the task.
	Score int
}

// ContextAt detects suggestion context at a rune cursor position.
func ContextAt(input string, cursor int) SuggestionContext {
	runes := []rune(input)
	cursor = max(0, min(cursor, len(runes)))
	start := cursor
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	end := cursor
	for end < len(runes) && !unicode.IsSpace(runes[end]) {
		end++
	}
	if start >= end {
		return SuggestionContext{Cursor: cursor, TokenStart: start, TokenEnd: end}
	}
	tokenRunes := runes[start:end]
	if len(tokenRunes) == 0 || (start > 0 && !unicode.IsSpace(runes[start-1])) {
		return SuggestionContext{Cursor: cursor, TokenStart: start, TokenEnd: end}
	}
	trigger := Trigger(tokenRunes[0])
	if !isTrigger(trigger) {
		return SuggestionContext{Cursor: cursor, TokenStart: start, TokenEnd: end}
	}
	if start+1 < len(runes) && runes[start] == '\\' {
		return SuggestionContext{Cursor: cursor, TokenStart: start, TokenEnd: end}
	}
	kind := kindForTrigger(trigger)
	if kind == "" {
		return SuggestionContext{Cursor: cursor, TokenStart: start, TokenEnd: end}
	}
	prefixEnd := cursor
	if prefixEnd < start+1 {
		prefixEnd = start + 1
	}
	return SuggestionContext{
		Active:     true,
		Kind:       kind,
		Trigger:    trigger,
		Token:      string(tokenRunes),
		Prefix:     string(runes[start+1 : prefixEnd]),
		Cursor:     cursor,
		TokenStart: start,
		TokenEnd:   end,
	}
}

// SuggestionContextAt is an alias retained for readable call sites.
func SuggestionContextAt(input string, cursor int) SuggestionContext {
	return ContextAt(input, cursor)
}

// SuggestionsFor returns field-aware, fuzzy-ranked candidates.
func SuggestionsFor(ctx SuggestionContext, projects, tags []string, now time.Time) []Suggestion {
	return SuggestionsWithProjectLabels(ctx, projects, tags, nil, now)
}

// SuggestionsWithProjectLabels matches both project values and readable labels.
// Completion always inserts the value, never the display label.
func SuggestionsWithProjectLabels(ctx SuggestionContext, projects, tags []string, labels map[string]string, now time.Time) []Suggestion {
	if !ctx.Active {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	values := make([]string, 0)
	switch ctx.Kind {
	case SuggestionProject:
		values = append(values, projects...)
	case SuggestionTag:
		values = append(values, tags...)
	case SuggestionPriority:
		values = []string{"high", "medium", "low", "none"}
	case SuggestionDue, SuggestionScheduled:
		values = dateSuggestions(now)
	case SuggestionEstimate:
		values = domain.EstimateSuggestions()
	case SuggestionRecurrence:
		values = []string{"daily", "weekdays", "weekly", "monday", "tuesday", "wednesday", "thursday", "friday", "monthly", "2wks"}
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]Suggestion, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		score, ok := fuzzyScore(ctx.Prefix, value)
		label := ""
		if ctx.Kind == SuggestionProject {
			label = labels[value]
			if label != "" {
				if labelScore, matches := fuzzyScore(ctx.Prefix, label); matches {
					if !ok || labelScore > score {
						score = labelScore
					}
					ok = true
				}
			}
		}
		if !ok {
			continue
		}
		result = append(result, Suggestion{
			Kind:  ctx.Kind,
			Value: value,
			Text:  string(ctx.Trigger) + value,
			Label: label,
			Score: score,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		// Keep source order for equal scores. Callers provide stable project/tag
		// lists, while fixed priority/date lists intentionally retain their
		// human-facing order.
		return result[i].Score > result[j].Score
	})
	return result
}

// ApplySuggestion replaces the complete token under the context and returns a
// rune cursor immediately after the inserted token.
func ApplySuggestion(input string, ctx SuggestionContext, suggestion Suggestion) (string, int) {
	if !ctx.Active {
		return input, ctx.Cursor
	}
	runes := []rune(input)
	start := max(0, min(ctx.TokenStart, len(runes)))
	end := max(start, min(ctx.TokenEnd, len(runes)))
	text := suggestion.Text
	if text == "" {
		text = string(ctx.Trigger) + suggestion.Value
	}
	updated := make([]rune, 0, len(runes)+len([]rune(text)))
	updated = append(updated, runes[:start]...)
	updated = append(updated, []rune(text)...)
	updated = append(updated, runes[end:]...)
	return string(updated), start + len([]rune(text))
}

func kindForTrigger(trigger Trigger) SuggestionKind {
	switch trigger {
	case TriggerProject:
		return SuggestionProject
	case TriggerPriority:
		return SuggestionPriority
	case TriggerDue:
		return SuggestionDue
	case TriggerScheduled:
		return SuggestionScheduled
	case TriggerTag:
		return SuggestionTag
	case TriggerEstimate:
		return SuggestionEstimate
	case TriggerRecurrence:
		return SuggestionRecurrence
	default:
		return ""
	}
}

func dateSuggestions(now time.Time) []string {
	loc := now.Location()
	date := now.In(loc)
	values := []string{"today", "tomorrow"}
	for offset := 1; offset <= 7; offset++ {
		candidate := date.AddDate(0, 0, offset)
		values = append(values, strings.ToLower(candidate.Weekday().String()))
	}
	values = append(values, "next-week", date.AddDate(0, 0, 7).Format("2006-01-02"))
	return values
}

func fuzzyScore(query, candidate string) (int, bool) {
	queryRunes := []rune(strings.ToLower(query))
	candidateRunes := []rune(strings.ToLower(candidate))
	if len(queryRunes) == 0 {
		return 0, true
	}
	if string(queryRunes) == string(candidateRunes) {
		return 10000, true
	}
	if len(queryRunes) <= len(candidateRunes) && string(candidateRunes[:len(queryRunes)]) == string(queryRunes) {
		return 8000 - (len(candidateRunes) - len(queryRunes)), true
	}
	queryIndex := 0
	score := 0
	last := -1
	for index, char := range candidateRunes {
		if queryIndex >= len(queryRunes) || char != queryRunes[queryIndex] {
			continue
		}
		if last+1 == index {
			score += 30
		} else {
			score += 10
		}
		if index == 0 || candidateRunes[index-1] == '.' || candidateRunes[index-1] == '-' || candidateRunes[index-1] == '_' {
			score += 20
		}
		last = index
		queryIndex++
	}
	if queryIndex != len(queryRunes) {
		return 0, false
	}
	return score - (len(candidateRunes) - len(queryRunes)), true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
