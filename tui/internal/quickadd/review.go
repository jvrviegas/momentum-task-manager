package quickadd

import (
	"fmt"
	"strings"
	"time"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/domain"
)

// Correction-review field indexes, in review form order. Every frontend
// presents and submits review values in this order.
const (
	ReviewDescription = iota
	ReviewDue
	ReviewRecurrence
	ReviewEstimate
	ReviewPriority
	ReviewProject
	ReviewScheduled
	ReviewTags
	ReviewFieldCount
)

// FinalizeReview validates reviewed values and returns the task to add.
// touched marks fields the user edited; decisions marks fields where the user
// explicitly kept the current candidate.
func FinalizeReview(review Interpretation, values []string, touched, decisions [ReviewFieldCount]bool) (domain.NewTask, error) {
	if err := ValidateReview(review, values, touched, decisions); err != nil {
		return domain.NewTask{}, err
	}
	if strings.TrimSpace(values[ReviewDescription]) == "" {
		return domain.NewTask{}, fmt.Errorf("task description cannot be empty")
	}
	if estimateValue := strings.TrimSpace(values[ReviewEstimate]); estimateValue != "" {
		if _, err := domain.ParseEstimate(estimateValue); err != nil {
			return domain.NewTask{}, err
		}
	}
	switch normalizeReviewPriority(values[ReviewPriority]) {
	case "", "H", "M", "L":
	default:
		return domain.NewTask{}, fmt.Errorf("priority must be H, M, L, or empty")
	}
	task := applyReviewValues(review.Task, values)
	if task.Recurrence != "" {
		recurrence, err := domain.ParseRecurrence(task.Recurrence)
		if err != nil {
			return domain.NewTask{}, err
		}
		task.Recurrence = recurrence
		if task.Due == "" {
			return domain.NewTask{}, fmt.Errorf("recurring tasks need a first due date")
		}
	}
	return task, nil
}

func ValidateReview(review Interpretation, values []string, touched, decisions [ReviewFieldCount]bool) error {
	if len(values) < ReviewFieldCount {
		return fmt.Errorf("review fields are incomplete")
	}
	original := ReviewValues(review.Task)
	semanticChanged := func(index int) bool {
		if index == ReviewRecurrence {
			return recurrenceValueChanged(original[index], values[index])
		}
		return strings.TrimSpace(values[index]) != strings.TrimSpace(original[index])
	}
	explicitClear := func(index int) bool {
		return touched[index] && strings.TrimSpace(original[index]) != "" && strings.TrimSpace(values[index]) == ""
	}
	fieldResolved := func(index int) bool {
		if decisions[index] || semanticChanged(index) || explicitClear(index) {
			return true
		}
		if touched[index] && strings.TrimSpace(original[index]) == "" && strings.TrimSpace(values[index]) == "" {
			for _, candidate := range review.Candidates {
				for _, candidateIndex := range ReviewFieldIndexes(candidate.Field) {
					if candidateIndex == index && candidateAllowsExplicitClear(candidate) {
						return true
					}
				}
			}
		}
		return false
	}
	candidateResolved := func(candidate Candidate, index int) bool {
		if fieldResolved(index) {
			return true
		}
		// Invalid recognized metadata can be deliberately cleared even when its
		// original task field was empty. A type/delete round trip is not such a
		// decision: the final semantic value is still unchanged.
		return touched[index] && strings.TrimSpace(original[index]) == "" && strings.TrimSpace(values[index]) == "" && candidateAllowsExplicitClear(candidate)
	}

	reference := review.ReferenceTime
	if reference.IsZero() {
		reference = time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	}
	for _, candidate := range review.Candidates {
		if !candidate.Blocking {
			continue
		}
		if candidate.Conflict && (candidate.Field == FieldDue || candidate.Field == FieldTime) {
			// The original recurrence anchor is already compatible with its
			// recurrence. The user must change the anchor semantically, change the
			// recurrence semantics, explicitly clear a field, or press Ctrl+R to
			// keep the candidate literal. Typing and deleting back to the same
			// value is not resolution.
			if decisions[ReviewDue] || decisions[ReviewRecurrence] || recurrenceValueChanged(original[ReviewRecurrence], values[ReviewRecurrence]) || explicitClear(ReviewDue) {
				continue
			}
			if !semanticChanged(ReviewDue) || !reviewedRecurrenceAnchorMatches(review, values[ReviewDue], reference) {
				return fmt.Errorf("resolve the highlighted interpretation or choose explicit syntax only")
			}
			continue
		}
		for _, index := range ReviewFieldIndexes(candidate.Field) {
			if !candidateResolved(candidate, index) {
				return fmt.Errorf("resolve the highlighted interpretation or choose explicit syntax only")
			}
		}
	}
	for _, diagnostic := range review.Diagnostics {
		if !diagnostic.Blocking {
			continue
		}
		indexes := diagnosticFieldIndexes(diagnostic.Message)
		if len(indexes) == 0 {
			resolved := false
			for index := range values[:ReviewFieldCount] {
				if fieldResolved(index) {
					resolved = true
					break
				}
			}
			if !resolved {
				return fmt.Errorf("resolve the highlighted interpretation or choose explicit syntax only")
			}
			continue
		}
		resolved := false
		for _, index := range indexes {
			if fieldResolved(index) {
				resolved = true
				break
			}
		}
		if !resolved {
			return fmt.Errorf("resolve the highlighted interpretation or choose explicit syntax only")
		}
	}
	if !review.Valid {
		resolved := false
		for index := range values[:ReviewFieldCount] {
			if fieldResolved(index) {
				resolved = true
				break
			}
		}
		if !resolved {
			return fmt.Errorf("resolve the highlighted interpretation or choose explicit syntax only")
		}
	}
	return nil
}

func ReviewFieldIndexes(field CandidateField) []int {
	switch field {
	case FieldDue, FieldTime:
		return []int{ReviewDue}
	case FieldRecurrence:
		return []int{ReviewRecurrence}
	case FieldEstimate:
		return []int{ReviewEstimate}
	case FieldPriority:
		return []int{ReviewPriority}
	case FieldProject:
		return []int{ReviewProject}
	case FieldScheduled:
		return []int{ReviewScheduled}
	case FieldTag:
		return []int{ReviewTags}
	default:
		return nil
	}
}

func recurrenceValueChanged(original, current string) bool {
	originalCanonical, originalErr := domain.ParseRecurrence(original)
	currentCanonical, currentErr := domain.ParseRecurrence(current)
	if originalErr == nil && currentErr == nil {
		return originalCanonical != currentCanonical
	}
	return strings.TrimSpace(original) != strings.TrimSpace(current)
}

func candidateAllowsExplicitClear(candidate Candidate) bool {
	text := strings.ToLower(candidate.Diagnostic)
	for _, marker := range []string{"invalid", "does not exist", "ambiguous", "incomplete", "needs a supported"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func reviewedRecurrenceAnchorMatches(review Interpretation, value string, reference time.Time) bool {
	original, ok := ReviewAnchor(review.Task.Due, reference)
	if !ok {
		return false
	}
	current, ok := ReviewAnchor(value, reference)
	return ok && current.Weekday() == original.Weekday()
}

func diagnosticFieldIndexes(message string) []int {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "estimate"), strings.Contains(text, "effort"):
		return []int{ReviewEstimate}
	case strings.Contains(text, "priority"):
		return []int{ReviewPriority}
	case strings.Contains(text, "recurrence"), strings.Contains(text, "anchor"):
		return []int{ReviewRecurrence, ReviewDue}
	case strings.Contains(text, "date"), strings.Contains(text, "time"), strings.Contains(text, "due"):
		return []int{ReviewDue}
	case strings.Contains(text, "description"):
		return []int{ReviewDescription}
	default:
		return nil
	}
}

func ReviewValues(task domain.NewTask) []string {
	estimate := ""
	if task.Estimate != nil {
		estimate = task.Estimate.InputValue()
	}
	return []string{
		task.Description,
		task.Due,
		task.Recurrence,
		estimate,
		task.Priority,
		task.Project,
		task.Scheduled,
		strings.Join(task.Tags, " "),
	}
}

func applyReviewValues(task domain.NewTask, values []string) domain.NewTask {
	task = cloneNewTask(task)
	task.Description = strings.TrimSpace(values[ReviewDescription])
	task.Due = strings.TrimSpace(values[ReviewDue])
	task.Recurrence = strings.TrimSpace(values[ReviewRecurrence])
	task.Estimate = nil
	if value := strings.TrimSpace(values[ReviewEstimate]); value != "" {
		if estimate, err := domain.ParseEstimate(value); err == nil {
			task.Estimate = &estimate
		}
	}
	task.Priority = normalizeReviewPriority(values[ReviewPriority])
	task.Project = strings.TrimSpace(values[ReviewProject])
	task.Scheduled = strings.TrimSpace(values[ReviewScheduled])
	task.Tags = splitReviewTags(values[ReviewTags])
	return task
}

func splitReviewTags(value string) []string {
	fields := strings.Fields(value)
	result := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		field = strings.TrimPrefix(strings.TrimSpace(field), "+")
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	return result
}

func ReviewAnchor(value string, now time.Time) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if parsed, err := time.ParseInLocation("20060102T150405", value, now.Location()); err == nil {
		return parsed, true
	}
	if parsed, err := domain.ParseTaskwarriorTime(value); err == nil {
		return parsed.In(now.Location()), true
	}
	for _, candidate := range []string{"today", "tomorrow"} {
		if strings.EqualFold(strings.TrimSpace(value), candidate) {
			offset := 0
			if candidate == "tomorrow" {
				offset = 1
			}
			base := now.In(now.Location())
			return time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()).AddDate(0, 0, offset), true
		}
	}
	return time.Time{}, false
}

func normalizeReviewPriority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none":
		return ""
	case "h", "high":
		return "H"
	case "m", "medium":
		return "M"
	case "l", "low":
		return "L"
	default:
		return strings.TrimSpace(value)
	}
}
