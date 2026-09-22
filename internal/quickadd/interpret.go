package quickadd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jvrviegas/momentum/internal/domain"
)

// CandidateProvenance explains why one draft field has a value.
type CandidateProvenance string

const (
	ProvenanceExplicit CandidateProvenance = "explicit"
	ProvenanceInferred CandidateProvenance = "inferred"
)

type CandidateField string

const (
	FieldDue        CandidateField = "due"
	FieldEstimate   CandidateField = "estimate"
	FieldPriority   CandidateField = "priority"
	FieldRecurrence CandidateField = "recurrence"
	FieldTime       CandidateField = "time"
	FieldProject    CandidateField = "project"
	FieldScheduled  CandidateField = "scheduled"
	FieldTag        CandidateField = "tag"
)

// Candidate records source accounting for one accepted or conflicting phrase.
// Start and End are rune offsets into Source, matching quick-add suggestions.
type Candidate struct {
	Field      CandidateField
	Text       string
	Value      string
	Start      int
	End        int
	Provenance CandidateProvenance
	Accepted   bool
	Conflict   bool
	Blocking   bool
	Diagnostic string
}

type Diagnostic struct {
	Message  string
	Blocking bool
}

// Interpretation is a reviewable, local draft. Task contains only values that
// are safe to submit; source text for rejected/conflicting phrases remains in
// Description and Diagnostics explains why.
type Interpretation struct {
	Source         string
	Task           domain.NewTask
	Candidates     []Candidate
	Diagnostics    []Diagnostic
	RequiresReview bool
	Valid          bool
	ExplicitOnly   domain.NewTask
	Timezone       string
	// ReferenceTime freezes the local clock/location used for this review.
	// Confirmation never reparses relative phrases against a later clock.
	ReferenceTime time.Time
}

var (
	// These expressions deliberately recognize a little more than the valid
	// grammar. A numeric time or effort clause is a candidate before it is
	// validated, so malformed input cannot silently become prose plus a
	// midnight/default value.
	naturalDateRE = regexp.MustCompile(`(?i)(?:next\s+(?:sunday|monday|tuesday|wednesday|thursday|friday|saturday)|today|tomorrow|(?:sunday|monday|tuesday|wednesday|thursday|friday|saturday)|\d{4}-\d{2}-\d{2}|in\s+\d+\s+days?)`)
	naturalTimeRE = regexp.MustCompile(`(?i)\bat\s+\d+(?::\d+)?(?:\s*(?:am|pm))?`)
	timeIntroRE   = regexp.MustCompile(`(?i)\bat\b`)
	priorityRE    = regexp.MustCompile(`(?i)p[123]`)
	recurrenceRE  = regexp.MustCompile(`(?i)every\s+(?:(?:\d+)\s+(?:days?|weeks?|months?)|weekday|day|week|month|sunday|monday|tuesday|wednesday|thursday|friday|saturday)`)

	unsupportedDateRE = regexp.MustCompile(`(?i)this\s+(?:sunday|monday|tuesday|wednesday|thursday|friday|saturday)`)
	// Reserve the whole unsupported recurrence clause. In particular, do not
	// let the weekday in "every other Friday" become a one-off due date.
	unsupportedRecurrenceRE = regexp.MustCompile(`(?i)every\s+(?:(?:other|second|third|fourth|\d+(?:st|nd|rd|th))\s+)(?:day|days|weekday|week|weeks|month|months|sunday|monday|tuesday|wednesday|thursday|friday|saturday)`)
	effortRE                = regexp.MustCompile(`(?i)\b(about|for)\s+((?:\d+(?:\.\d+)?h(?:\d+m)?|\d+(?:\.\d+)?\s*(?:hours?|h)|\d+(?:\.\d+)?\s*(?:minutes?|m)|an?\s+hour|an?\s+minute))`)
	urlRE                   = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s]+`)
	emailRE                 = regexp.MustCompile(`(?i)[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
)

type sourceSpan struct{ start, end int }

// Interpret applies the deliberately small English grammar approved for
// natural-language capture. It performs no I/O and never invokes Taskwarrior.
func Interpret(input string, now time.Time) (Interpretation, error) {
	if now.IsZero() {
		now = time.Now()
	}
	if now.Location() == nil {
		now = now.In(time.Local)
	}

	explicit, err := Parse(input)
	if err != nil {
		return Interpretation{}, err
	}

	result := Interpretation{
		Source:        input,
		ExplicitOnly:  cloneNewTask(explicit),
		Task:          cloneNewTask(explicit),
		Valid:         true,
		Timezone:      now.Location().String(),
		ReferenceTime: now,
	}
	explicitSpans, protected := explicitSourceSpans(input)
	protected = append(protected, protectedTextSpans(input)...)
	reserved := append([]sourceSpan(nil), matchedSpans(findMatches(unsupportedDateRE, input))...)
	unsupportedRecurrences := naturalMatches(unsupportedRecurrenceRE, input, protected, nil)
	for _, match := range unsupportedRecurrences {
		reserved = append(reserved, match.span)
	}

	consumed := append([]sourceSpan(nil), explicitSpans...)
	explicitFields := explicitFieldPresence(input)
	var naturalCandidates []Candidate
	var recurrenceAnchor *time.Time
	recurrenceWeekday := false
	recurrenceAccepted := false
	recurrenceDateConflict := false

	// Unsupported reserved recurrence phrases are intentionally literal. They
	// still enter review so the user can see why no recurrence was created.
	for _, match := range unsupportedRecurrences {
		result.RequiresReview = true
		naturalCandidates = append(naturalCandidates, Candidate{
			Field:      FieldRecurrence,
			Text:       match.text,
			Start:      match.span.start,
			End:        match.span.end,
			Provenance: ProvenanceInferred,
			Diagnostic: "unsupported recurrence phrase remains literal; choose explicit syntax or a supported recurrence",
		})
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Message:  "Unsupported recurrence remains in the task description; no recurrence will be created.",
			Blocking: false,
		})
	}

	// Recurrence is recognized before dates so a weekday belonging to a
	// recurrence clause is never also inferred as a one-off deadline.
	recurrenceMatches := naturalMatches(recurrenceRE, input, protected, reserved)
	type recurrenceItem struct {
		candidate Candidate
		spec      domain.RecurrenceDefinition
		err       error
	}
	recurrenceItems := make([]recurrenceItem, 0, len(recurrenceMatches))
	for _, match := range recurrenceMatches {
		result.RequiresReview = true
		candidate := Candidate{Field: FieldRecurrence, Text: match.text, Start: match.span.start, End: match.span.end, Provenance: ProvenanceInferred}
		if explicitFields[TriggerRecurrence] {
			candidate.Conflict = true
			candidate.Diagnostic = "explicit recurrence takes precedence"
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "Explicit recurrence takes precedence; the inferred recurrence remains description text.", Blocking: false})
			recurrenceItems = append(recurrenceItems, recurrenceItem{candidate: candidate})
			continue
		}
		spec, parseErr := domain.ParseRecurrencePhrase(match.text, now)
		if parseErr != nil {
			candidate.Diagnostic = parseErr.Error()
			candidate.Blocking = true
			result.Valid = false
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: parseErr.Error(), Blocking: true})
			recurrenceItems = append(recurrenceItems, recurrenceItem{candidate: candidate, err: parseErr})
			continue
		}
		recurrenceItems = append(recurrenceItems, recurrenceItem{candidate: candidate, spec: spec})
	}
	validRecurrences := 0
	validRecurrenceIndex := -1
	for index, item := range recurrenceItems {
		if item.err == nil && item.spec.Expression != "" && !item.candidate.Conflict {
			validRecurrences++
			validRecurrenceIndex = index
		}
	}
	if !explicitFields[TriggerRecurrence] && validRecurrences > 1 {
		result.Valid = false
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "multiple inferred recurrences found; keep one recurrence phrase", Blocking: true})
	} else if explicitFields[TriggerRecurrence] && len(recurrenceItems) > 1 {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "Explicit recurrence takes precedence over multiple inferred recurrence phrases.", Blocking: false})
	}
	for index, item := range recurrenceItems {
		candidate := item.candidate
		if explicitFields[TriggerRecurrence] {
			naturalCandidates = append(naturalCandidates, candidate)
			continue
		}
		if validRecurrences == 1 && index == validRecurrenceIndex && item.err == nil {
			candidate.Value = item.spec.Expression
			candidate.Accepted = true
			naturalCandidates = append(naturalCandidates, candidate)
			result.Task.Recurrence = item.spec.Expression
			anchor := item.spec.Anchor
			recurrenceAnchor = &anchor
			recurrenceAccepted = true
			recurrenceText := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(item.candidate.Text), "every "))
			_, recurrenceWeekday = parseWeekdayValue(recurrenceText)
			consumed = append(consumed, ownedSpan(input, sourceSpan{candidate.Start, candidate.End}))
			continue
		}
		if len(recurrenceItems) > 1 && item.err == nil {
			candidate.Blocking = true
			candidate.Diagnostic = "multiple inferred recurrence candidates"
			result.Valid = false
		}
		naturalCandidates = append(naturalCandidates, candidate)
	}

	// A recurrence supplies its native anchor unless an explicit due value or
	// a single compatible natural date supplies the first due date.
	if recurrenceAccepted && recurrenceAnchor != nil && !explicitFields[TriggerDue] {
		result.Task.Due = formatInferredDate(*recurrenceAnchor)
	}

	dateMatches := naturalMatches(naturalDateRE, input, protected, reserved)
	dates := make([]dateItem, 0, len(dateMatches))
	for _, match := range dateMatches {
		if overlapsAny(match.span, spansForCandidates(naturalCandidates, FieldRecurrence)) {
			continue
		}
		result.RequiresReview = true
		candidate := Candidate{Field: FieldDue, Text: match.text, Start: match.span.start, End: match.span.end, Provenance: ProvenanceInferred}
		resolved, parseErr := resolveNaturalDate(match.text, now)
		if parseErr != nil {
			candidate.Diagnostic = parseErr.Error()
			candidate.Blocking = !explicitFields[TriggerDue]
			if candidate.Blocking {
				result.Valid = false
			}
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: parseErr.Error(), Blocking: candidate.Blocking})
			dates = append(dates, dateItem{candidate: candidate, err: parseErr})
			continue
		}
		dates = append(dates, dateItem{candidate: candidate, value: resolved})
	}
	validDates := 0
	validDateIndex := -1
	for index, item := range dates {
		if item.err == nil {
			validDates++
			validDateIndex = index
		}
	}
	dateAccepted := false
	if validDates > 1 || (validDates > 0 && len(dates) != validDates) {
		blocking := !explicitFields[TriggerDue]
		if blocking {
			result.Valid = false
		}
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "multiple or invalid inferred dates found; keep one date phrase or use an explicit @due value", Blocking: blocking})
		for index := range dates {
			if dates[index].err == nil {
				dates[index].candidate.Blocking = blocking
				dates[index].candidate.Diagnostic = "date candidate needs an explicit selection"
			}
		}
	} else if validDates == 1 {
		item := dates[validDateIndex]
		candidate := item.candidate
		candidate.Value = formatInferredDate(item.value)
		if explicitFields[TriggerDue] {
			candidate.Conflict = true
			candidate.Diagnostic = "explicit due takes precedence"
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "Explicit @due takes precedence; the inferred date remains description text.", Blocking: false})
		} else if recurrenceAccepted && recurrenceWeekday && recurrenceAnchor != nil && item.value.Weekday() != recurrenceAnchor.Weekday() {
			recurrenceDateConflict = true
			candidate.Conflict = true
			candidate.Blocking = true
			candidate.Diagnostic = "date conflicts with the selected recurrence weekday"
			result.Valid = false
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "The first due date conflicts with the recurrence weekday; correct the anchor before confirming.", Blocking: true})
		} else {
			candidate.Accepted = true
			dateAccepted = true
			result.Task.Due = formatInferredDate(item.value)
			consumed = append(consumed, ownedSpan(input, item.candidateSpan()))
		}
		naturalCandidates = append(naturalCandidates, candidate)
	} else if len(dates) > 1 {
		blocking := !explicitFields[TriggerDue]
		if blocking {
			result.Valid = false
		}
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "multiple inferred dates found; keep one date phrase or use an explicit @due value", Blocking: blocking})
	}
	if len(dates) > 1 {
		for _, item := range dates {
			candidate := item.candidate
			if candidate.Diagnostic == "" {
				candidate.Diagnostic = "multiple inferred date candidates"
			}
			candidate.Blocking = !explicitFields[TriggerDue]
			if candidate.Blocking {
				result.Valid = false
			}
			naturalCandidates = append(naturalCandidates, candidate)
		}
	} else if len(dates) == 1 && validDates == 0 {
		naturalCandidates = append(naturalCandidates, dates[0].candidate)
	}

	// Numeric time phrases are candidates even when their hour/minute or
	// meridiem is invalid. A bare time never chooses a date implicitly.
	timeMatches := naturalMatches(naturalTimeRE, input, protected, reserved)
	for _, match := range incompleteTimeMatches(input, protected, reserved, timeMatches) {
		timeMatches = append(timeMatches, match)
	}
	times := make([]timeItem, 0, len(timeMatches))
	for _, match := range timeMatches {
		result.RequiresReview = true
		candidate := Candidate{Field: FieldTime, Text: match.text, Start: match.span.start, End: match.span.end, Provenance: ProvenanceInferred}
		clock, parseErr := parseNaturalTime(match.text, now.Location())
		if parseErr != nil {
			candidate.Diagnostic = parseErr.Error()
			candidate.Blocking = !explicitFields[TriggerDue]
			if candidate.Blocking {
				result.Valid = false
			}
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: parseErr.Error(), Blocking: candidate.Blocking})
			times = append(times, timeItem{candidate: candidate, err: parseErr})
			continue
		}
		times = append(times, timeItem{candidate: candidate, value: clock})
	}
	validTimes := 0
	for _, item := range times {
		if item.err == nil {
			validTimes++
		}
	}
	if len(times) > 1 {
		if !explicitFields[TriggerDue] {
			result.Valid = false
		}
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "multiple inferred times found; keep one time phrase", Blocking: !explicitFields[TriggerDue]})
		for index := range times {
			candidate := times[index].candidate
			if candidate.Diagnostic == "" {
				candidate.Diagnostic = "multiple inferred time candidates"
			}
			candidate.Blocking = !explicitFields[TriggerDue]
			naturalCandidates = append(naturalCandidates, candidate)
		}
	} else if len(times) == 1 {
		item := times[0]
		candidate := item.candidate
		if item.err == nil && validTimes == 1 {
			switch {
			case explicitFields[TriggerDue]:
				candidate.Conflict = true
				candidate.Diagnostic = "explicit due prevents combining an inferred time"
				result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "The explicit @due value remains unchanged; the inferred time remains description text.", Blocking: false})
			case recurrenceDateConflict:
				candidate.Conflict = true
				candidate.Blocking = true
				candidate.Diagnostic = "time remains with the conflicting date until the anchor is corrected"
				result.Valid = false
				result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "Correct the recurrence anchor before confirming the inferred time.", Blocking: true})
			case dateAccepted:
				date := dates[validDateIndex].value
				resolved, wallErr := resolveWallClock(date, item.value.Hour(), item.value.Minute(), now.Location())
				if wallErr != nil {
					candidate.Blocking = true
					candidate.Diagnostic = wallErr.Error()
					result.Valid = false
					result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: wallErr.Error(), Blocking: true})
				} else {
					candidate.Value = formatInferredDate(resolved)
					candidate.Accepted = true
					result.Task.Due = candidate.Value
					consumed = append(consumed, ownedSpan(input, sourceSpan{candidate.Start, candidate.End}))
				}
			case recurrenceAccepted && recurrenceAnchor != nil && !explicitFields[TriggerDue]:
				resolved, wallErr := resolveWallClock(*recurrenceAnchor, item.value.Hour(), item.value.Minute(), now.Location())
				if wallErr != nil {
					candidate.Blocking = true
					candidate.Diagnostic = wallErr.Error()
					result.Valid = false
					result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: wallErr.Error(), Blocking: true})
				} else {
					candidate.Value = formatInferredDate(resolved)
					candidate.Accepted = true
					result.Task.Due = candidate.Value
					consumed = append(consumed, ownedSpan(input, sourceSpan{candidate.Start, candidate.End}))
				}
			default:
				candidate.Blocking = true
				candidate.Diagnostic = "a time needs a supported date phrase"
				result.Valid = false
				result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "A bare time needs a date; add today/tomorrow/a weekday or use explicit @due syntax.", Blocking: true})
			}
		} else if item.err != nil {
			// The parse error and diagnostic were added above.
		} else {
			candidate.Blocking = !explicitFields[TriggerDue]
			candidate.Diagnostic = "time candidate needs an explicit selection"
			if candidate.Blocking {
				result.Valid = false
			}
		}
		naturalCandidates = append(naturalCandidates, candidate)
	}

	// Estimate phrases share the G0 parser and are resolved as a group before
	// any source span is consumed. This prevents first/last-wins behavior.
	effortMatches := naturalMatches(effortRE, input, protected, reserved)
	effortItems := make([]effortItem, 0, len(effortMatches))
	for _, match := range effortMatches {
		result.RequiresReview = true
		candidate := Candidate{Field: FieldEstimate, Text: match.text, Start: match.span.start, End: match.span.end, Provenance: ProvenanceInferred}
		if len(match.groups) < 2 {
			candidate.Blocking = !explicitFields[TriggerEstimate]
			candidate.Diagnostic = "invalid effort phrase"
			effortItems = append(effortItems, effortItem{candidate: candidate, err: fmt.Errorf("invalid effort phrase")})
			continue
		}
		value, parseErr := effortValue(match.groups[0], match.groups[1])
		if parseErr == nil {
			var estimate domain.Estimate
			estimate, parseErr = domain.ParseEstimate(value)
			if parseErr == nil {
				effortItems = append(effortItems, effortItem{candidate: candidate, estimate: estimate})
				continue
			}
		}
		candidate.Diagnostic = parseErr.Error()
		candidate.Blocking = !explicitFields[TriggerEstimate]
		if candidate.Blocking {
			result.Valid = false
		}
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: parseErr.Error(), Blocking: candidate.Blocking})
		effortItems = append(effortItems, effortItem{candidate: candidate, err: parseErr})
	}
	validEfforts := 0
	validEffortIndex := -1
	for index, item := range effortItems {
		if item.err == nil {
			validEfforts++
			validEffortIndex = index
		}
	}
	for index, item := range effortItems {
		candidate := item.candidate
		if explicitFields[TriggerEstimate] {
			candidate.Conflict = true
			if candidate.Diagnostic == "" {
				candidate.Diagnostic = "explicit estimate takes precedence"
			}
			if item.err == nil {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "Explicit ~estimate takes precedence; the inferred effort remains description text.", Blocking: false})
			}
			naturalCandidates = append(naturalCandidates, candidate)
			continue
		}
		if validEfforts == 1 && index == validEffortIndex && item.err == nil {
			candidate.Value = item.estimate.String()
			candidate.Accepted = true
			result.Task.Estimate = cloneEstimate(item.estimate)
			consumed = append(consumed, ownedSpan(input, sourceSpan{candidate.Start, candidate.End}))
		} else if len(effortItems) > 1 && item.err == nil {
			candidate.Blocking = true
			candidate.Diagnostic = "multiple inferred effort candidates"
			result.Valid = false
		}
		naturalCandidates = append(naturalCandidates, candidate)
	}
	if validEfforts > 1 && !explicitFields[TriggerEstimate] {
		result.Valid = false
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "multiple inferred estimates found; keep one effort phrase", Blocking: true})
	}

	priorityMatches := naturalMatches(priorityRE, input, protected, reserved)
	priorityCandidates := make([]Candidate, 0, len(priorityMatches))
	for _, match := range priorityMatches {
		result.RequiresReview = true
		candidate := Candidate{Field: FieldPriority, Text: match.text, Value: strings.ToUpper(match.text), Start: match.span.start, End: match.span.end, Provenance: ProvenanceInferred}
		priorityCandidates = append(priorityCandidates, candidate)
	}
	if explicitFields[TriggerPriority] {
		for _, candidate := range priorityCandidates {
			candidate.Conflict = true
			candidate.Diagnostic = "explicit priority takes precedence"
			naturalCandidates = append(naturalCandidates, candidate)
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "Explicit !priority takes precedence; the inferred shorthand remains description text.", Blocking: false})
		}
	} else if len(priorityCandidates) == 1 {
		candidate := priorityCandidates[0]
		candidate.Value = mapPriorityShorthand(candidate.Text)
		candidate.Accepted = true
		result.Task.Priority = candidate.Value
		consumed = append(consumed, ownedSpan(input, sourceSpan{candidate.Start, candidate.End}))
		naturalCandidates = append(naturalCandidates, candidate)
	} else if len(priorityCandidates) > 1 {
		result.Valid = false
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "multiple inferred priorities found; keep one priority shorthand", Blocking: true})
		for _, candidate := range priorityCandidates {
			candidate.Blocking = true
			candidate.Diagnostic = "multiple inferred priority candidates"
			naturalCandidates = append(naturalCandidates, candidate)
		}
	}

	result.Task.Description = reconstructDescription(input, consumed)
	if strings.TrimSpace(result.Task.Description) == "" {
		result.Valid = false
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: "task description cannot be empty after interpretation", Blocking: true})
	}

	// Keep explicit provenance visible whenever a review was triggered, while
	// retaining the existing plain/explicit-only fast path when it was not.
	if result.RequiresReview {
		explicitCandidates := explicitCandidates(input, result.Task)
		result.Candidates = append(explicitCandidates, naturalCandidates...)
	} else {
		result.Candidates = nil
	}
	return result, nil
}

type matched struct {
	span   sourceSpan
	text   string
	groups []string
}

type dateItem struct {
	candidate Candidate
	value     time.Time
	err       error
}

func (d dateItem) candidateSpan() sourceSpan {
	return sourceSpan{start: d.candidate.Start, end: d.candidate.End}
}

type timeItem struct {
	candidate Candidate
	value     time.Time
	err       error
}

type effortItem struct {
	candidate Candidate
	estimate  domain.Estimate
	err       error
}

func findMatches(pattern *regexp.Regexp, input string) []matched {
	matches := pattern.FindAllStringSubmatchIndex(input, -1)
	result := make([]matched, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		start, end := byteToRuneOffset(input, match[0]), byteToRuneOffset(input, match[1])
		groups := make([]string, 0)
		for index := 2; index+1 < len(match); index += 2 {
			if match[index] < 0 || match[index+1] < 0 {
				groups = append(groups, "")
				continue
			}
			groups = append(groups, input[match[index]:match[index+1]])
		}
		result = append(result, matched{span: sourceSpan{start: start, end: end}, text: input[match[0]:match[1]], groups: groups})
	}
	return result
}

func naturalMatches(pattern *regexp.Regexp, input string, protected, reserved []sourceSpan) []matched {
	matches := findMatches(pattern, input)
	result := make([]matched, 0, len(matches))
	for _, match := range matches {
		if !isStandalonePhrase(input, match.span) || overlapsAny(match.span, protected) || overlapsAny(match.span, reserved) {
			continue
		}
		result = append(result, match)
	}
	return result
}

// incompleteTimeMatches catches a trailing "at"/"at pm" clause without
// treating ordinary prose such as "at the office" as a time candidate.
func incompleteTimeMatches(input string, protected, reserved []sourceSpan, existing []matched) []matched {
	runes := []rune(input)
	result := make([]matched, 0)
	for _, intro := range findMatches(timeIntroRE, input) {
		span := intro.span
		end := span.end
		for end < len(runes) && unicode.IsSpace(runes[end]) {
			end++
		}
		wordStart := end
		for end < len(runes) && unicode.IsLetter(runes[end]) {
			end++
		}
		word := strings.ToLower(string(runes[wordStart:end]))
		if word != "am" && word != "pm" {
			end = span.end
		}
		candidateSpan := sourceSpan{span.start, end}
		if !isStandalonePhrase(input, candidateSpan) || overlapsAny(candidateSpan, protected) || overlapsAny(candidateSpan, reserved) {
			continue
		}
		if end == span.end && word != "" {
			continue
		}
		if end < len(runes) && phraseBoundaryRune(runes[end]) == false {
			continue
		}
		overlapsExisting := false
		for _, current := range existing {
			if overlapsAny(candidateSpan, []sourceSpan{current.span}) {
				overlapsExisting = true
				break
			}
		}
		if overlapsExisting {
			continue
		}
		result = append(result, matched{span: candidateSpan, text: string(runes[candidateSpan.start:candidateSpan.end])})
	}
	return result
}

func explicitCandidates(input string, task domain.NewTask) []Candidate {
	candidates := make([]Candidate, 0)
	for _, token := range splitTokens(input) {
		if _, escaped := unescapeTrigger(token.text); escaped {
			continue
		}
		runes := []rune(token.text)
		if len(runes) == 0 || !isTrigger(Trigger(runes[0])) {
			continue
		}
		trigger := Trigger(runes[0])
		candidate := Candidate{Text: token.text, Start: token.start, End: token.end, Provenance: ProvenanceExplicit, Accepted: true}
		switch trigger {
		case TriggerProject:
			candidate.Field, candidate.Value = FieldProject, task.Project
		case TriggerPriority:
			candidate.Field, candidate.Value = FieldPriority, task.Priority
		case TriggerDue:
			candidate.Field, candidate.Value = FieldDue, task.Due
		case TriggerScheduled:
			candidate.Field, candidate.Value = FieldScheduled, task.Scheduled
		case TriggerEstimate:
			candidate.Field = FieldEstimate
			if task.Estimate != nil {
				candidate.Value = task.Estimate.String()
			}
		case TriggerRecurrence:
			candidate.Field, candidate.Value = FieldRecurrence, task.Recurrence
		case TriggerTag:
			candidate.Field = FieldTag
			candidate.Value = string(runes[1:])
		default:
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func protectedTextSpans(input string) []sourceSpan {
	var spans []sourceSpan
	for _, pattern := range []*regexp.Regexp{urlRE, emailRE} {
		for _, match := range findMatches(pattern, input) {
			spans = append(spans, match.span)
		}
	}
	return spans
}

func explicitSourceSpans(input string) ([]sourceSpan, []sourceSpan) {
	spans := make([]sourceSpan, 0)
	protected := make([]sourceSpan, 0)
	for _, token := range splitTokens(input) {
		if _, escaped := unescapeTrigger(token.text); escaped {
			protected = append(protected, sourceSpan{token.start, token.end})
			continue
		}
		runes := []rune(token.text)
		if len(runes) > 0 && isTrigger(Trigger(runes[0])) {
			span := sourceSpan{token.start, token.end}
			spans = append(spans, span)
			protected = append(protected, span)
		}
	}
	return spans, protected
}

func explicitFieldPresence(input string) map[Trigger]bool {
	fields := make(map[Trigger]bool)
	for _, token := range splitTokens(input) {
		if _, escaped := unescapeTrigger(token.text); escaped {
			continue
		}
		runes := []rune(token.text)
		if len(runes) > 0 && isTrigger(Trigger(runes[0])) {
			fields[Trigger(runes[0])] = true
		}
	}
	return fields
}

func matchedSpans(matches []matched) []sourceSpan {
	result := make([]sourceSpan, 0, len(matches))
	for _, match := range matches {
		result = append(result, match.span)
	}
	return result
}

func spansForCandidates(candidates []Candidate, field CandidateField) []sourceSpan {
	result := make([]sourceSpan, 0)
	for _, candidate := range candidates {
		if candidate.Field == field && candidate.End > candidate.Start {
			result = append(result, sourceSpan{candidate.Start, candidate.End})
		}
	}
	return result
}

func overlapsAny(value sourceSpan, spans []sourceSpan) bool {
	for _, other := range spans {
		if value.start < other.end && other.start < value.end {
			return true
		}
	}
	return false
}

func ownedSpan(input string, span sourceSpan) sourceSpan {
	runes := []rune(input)
	if span.end < len(runes) && runes[span.end] == ',' {
		span.end++
	}
	return span
}

func isStandalonePhrase(input string, span sourceSpan) bool {
	runes := []rune(input)
	if span.start < 0 || span.end > len(runes) || span.start >= span.end {
		return false
	}
	if span.start > 0 && !phraseBoundaryRune(runes[span.start-1]) {
		return false
	}
	if span.end < len(runes) && !phraseBoundaryRune(runes[span.end]) {
		return false
	}
	return true
}

func phraseBoundaryRune(value rune) bool {
	if unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_' || value == '-' || value == '/' || value == '@' || value == '#' || value == '+' || value == '~' || value == '>' || value == '^' {
		return false
	}
	return true
}

func reconstructDescription(input string, consumed []sourceSpan) string {
	runes := []rune(input)
	masked := make([]bool, len(runes))
	for _, span := range consumed {
		start, end := span.start, span.end
		if start < 0 {
			start = 0
		}
		if end > len(runes) {
			end = len(runes)
		}
		for index := start; index < end; index++ {
			masked[index] = true
		}
	}
	// Explicit escaped triggers are literal prose, with only the escape slash
	// removed. All other consumed tokens are removed.
	var parts []string
	for _, token := range splitTokens(input) {
		if token.start >= len(runes) {
			continue
		}
		if escaped, ok := unescapeTrigger(token.text); ok {
			parts = append(parts, escaped)
			continue
		}
		if allMasked(masked, token.start, token.end) {
			continue
		}
		value := make([]rune, 0, token.end-token.start)
		for index := token.start; index < token.end && index < len(runes); index++ {
			if !masked[index] {
				value = append(value, runes[index])
			}
		}
		if len(value) > 0 {
			parts = append(parts, string(value))
		}
	}
	return strings.Join(parts, " ")
}

func allMasked(values []bool, start, end int) bool {
	for index := start; index < end && index < len(values); index++ {
		if !values[index] {
			return false
		}
	}
	return true
}

func resolveNaturalDate(value string, now time.Time) (time.Time, error) {
	text := strings.ToLower(strings.TrimSpace(value))
	loc := now.Location()
	base := midnight(now.In(loc))
	switch text {
	case "today":
		return base, nil
	case "tomorrow":
		return base.AddDate(0, 0, 1), nil
	}
	if strings.HasPrefix(text, "next ") {
		weekday, ok := parseWeekdayValue(strings.TrimSpace(strings.TrimPrefix(text, "next ")))
		if !ok {
			return time.Time{}, fmt.Errorf("unsupported date phrase %q", value)
		}
		wanted, _ := weekdayNumber(weekday)
		monday := base.AddDate(0, 0, -((int(base.Weekday()) + 6) % 7))
		return monday.AddDate(0, 0, 7+((int(wanted)+6)%7)), nil
	}
	if weekday, ok := parseWeekdayValue(text); ok {
		wanted, _ := weekdayNumber(weekday)
		return nextWeekday(base, wanted), nil
	}
	if strings.HasPrefix(text, "in ") && (strings.HasSuffix(text, " day") || strings.HasSuffix(text, " days")) {
		parts := strings.Fields(text)
		if len(parts) == 3 {
			count, err := strconv.Atoi(parts[1])
			if err == nil && count > 0 {
				return base.AddDate(0, 0, count), nil
			}
		}
		return time.Time{}, fmt.Errorf("unsupported or invalid date phrase %q", value)
	}
	if parsed, err := time.ParseInLocation("2006-01-02", text, loc); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("unsupported or invalid date phrase %q", value)
}

func weekdayNumber(value string) (time.Weekday, bool) {
	weekdays := map[string]time.Weekday{"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday, "thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday}
	weekday, ok := weekdays[strings.ToLower(value)]
	return weekday, ok
}

func nextWeekday(from time.Time, wanted time.Weekday) time.Time {
	from = midnight(from)
	delta := (int(wanted) - int(from.Weekday()) + 7) % 7
	return from.AddDate(0, 0, delta)
}

func midnight(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

// resolveWallClock enumerates the offsets in a window around the requested
// civil time and converts each possible offset back to the location. This
// catches folds of any size (including 30-minute transitions) without relying
// on which occurrence time.Date chooses, and rejects gaps when no exact wall
// time exists.
func resolveWallClock(date time.Time, hour, minute int, location *time.Location) (time.Time, error) {
	if location == nil {
		location = time.Local
	}
	base := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, location)
	wall := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, time.UTC)
	wallNanos := wall.UnixNano()
	const searchWindow = 48 * time.Hour
	const sampleStep = 30 * time.Minute
	offsets := map[int]struct{}{}
	for offset := -searchWindow; offset <= searchWindow; offset += sampleStep {
		instant := base.Add(offset)
		_, zoneOffset := instant.In(location).Zone()
		offsets[zoneOffset] = struct{}{}
	}
	_, baseOffset := base.In(location).Zone()
	offsets[baseOffset] = struct{}{}

	matches := make([]time.Time, 0, 2)
	seen := make(map[int64]struct{})
	for offset := range offsets {
		instant := time.Unix(0, wallNanos-int64(offset)*int64(time.Second)).In(location)
		year, month, day := instant.Date()
		if year != date.Year() || month != date.Month() || day != date.Day() || instant.Hour() != hour || instant.Minute() != minute || instant.Second() != 0 {
			continue
		}
		key := instant.UnixNano()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		matches = append(matches, instant)
	}
	if len(matches) == 0 {
		return time.Time{}, fmt.Errorf("%s %02d:%02d does not exist in %s; correct the time", date.Format("2006-01-02"), hour, minute, location)
	}
	if len(matches) > 1 {
		return time.Time{}, fmt.Errorf("%s %02d:%02d is ambiguous in %s; correct the time", date.Format("2006-01-02"), hour, minute, location)
	}
	return matches[0], nil
}

func parseNaturalTime(value string, location *time.Location) (time.Time, error) {
	text := strings.ToLower(strings.TrimSpace(value))
	text = strings.TrimSpace(strings.TrimPrefix(text, "at "))
	for _, layout := range []string{"3:04pm", "3pm", "15:04"} {
		if parsed, err := time.ParseInLocation(layout, strings.ReplaceAll(text, " ", ""), location); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid or incomplete time phrase %q; use at 3pm, at 3:30pm, or at 15:00", value)
}

func effortValue(prefix, value string) (string, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "an hour" || value == "a hour" {
		return "1h", nil
	}
	if value == "an minute" || value == "a minute" {
		return "1m", nil
	}
	value = strings.ReplaceAll(value, "hours", "h")
	value = strings.ReplaceAll(value, "hour", "h")
	value = strings.ReplaceAll(value, "minutes", "m")
	value = strings.ReplaceAll(value, "minute", "m")
	value = strings.ReplaceAll(value, " ", "")
	if prefix != "about" && prefix != "for" {
		return "", fmt.Errorf("unsupported effort phrase")
	}
	return value, nil
}

func mapPriorityShorthand(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "p1":
		return "H"
	case "p2":
		return "M"
	case "p3":
		return "L"
	default:
		return ""
	}
}

func formatInferredDate(value time.Time) string {
	return value.Format("20060102T150405")
}

func byteToRuneOffset(value string, offset int) int {
	if offset <= 0 {
		return 0
	}
	if offset >= len(value) {
		return len([]rune(value))
	}
	return len([]rune(value[:offset]))
}

func cloneEstimate(value domain.Estimate) *domain.Estimate {
	return &domain.Estimate{Minutes: value.Minutes}
}

func cloneNewTask(value domain.NewTask) domain.NewTask {
	result := value
	result.Tags = append([]string(nil), value.Tags...)
	if value.Estimate != nil {
		result.Estimate = cloneEstimate(*value.Estimate)
	}
	return result
}
