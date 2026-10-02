package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
	"github.com/jvrviegas/momentum-task-manager/internal/quickadd"
)

const (
	quickAddReviewDescription = iota
	quickAddReviewDue
	quickAddReviewRecurrence
	quickAddReviewEstimate
	quickAddReviewPriority
	quickAddReviewProject
	quickAddReviewScheduled
	quickAddReviewTags
	quickAddReviewFieldCount
)

var quickAddReviewFieldNames = [...]string{
	"Description", "Due", "Recurrence", "Estimate", "Priority", "Project", "Scheduled", "Tags",
}

func (q *QuickAddModel) openReview(value quickadd.Interpretation) {
	q.Review = cloneInterpretation(value)
	q.OriginalInput = value.Source
	q.ReviewOpen = true
	q.ReviewRevision = q.CaptureRevision
	q.ReviewField = quickAddReviewDescription
	q.ReviewScroll = 0
	q.ReviewDetailScroll = 0
	q.ReviewTouched = [quickAddReviewFieldCount]bool{}
	q.ReviewDecisions = [quickAddReviewFieldCount]bool{}
	q.ParseErr = nil
	q.setReviewInputValues(q.Review.Task)
	q.focusReviewField(quickAddReviewDescription)
}

func (q *QuickAddModel) setReviewInputValues(task domain.NewTask) {
	values := reviewTaskValues(task)
	for index := range q.ReviewInputs {
		q.ReviewInputs[index].SetValue(values[index])
		q.ReviewInputs[index].Blur()
	}
	q.ensureReviewFieldVisible()
}

func (q *QuickAddModel) updateReview(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "esc", "escape":
		// Going back invalidates the reviewed draft. Reinterpretation will use a
		// fresh reference time and a new capture revision.
		q.CaptureRevision++
		q.ReviewOpen = false
		q.ParseErr = nil
		q.Input.SetValue(q.OriginalInput)
		q.Input.CursorEnd()
		q.refreshSuggestions()
		return q.Input.Focus()
	case "tab", "shift+tab":
		delta := 1
		if key.String() == "shift+tab" {
			delta = -1
		}
		q.focusReviewField(q.ReviewField + delta)
		return nil
	case "ctrl+s":
		return q.submitReview()
	case "ctrl+r":
		// Explicitly keep the current candidate literal/unchanged. This is
		// distinct from typing and deleting back to the same value.
		q.ReviewDecisions[q.ReviewField] = true
		q.ReviewDetailScroll = 0
		q.CaptureRevision++
		return nil
	case "ctrl+x":
		// Ctrl+X is deliberately distinct from ordinary x so descriptions such
		// as "x-ray" remain editable in every review field.
		input := q.OriginalInput
		revision := q.CaptureRevision
		return func() tea.Msg {
			task, err := quickadd.Parse(input)
			if err != nil {
				return QuickAddErrorMsg{Err: err, Revision: revision, Source: input}
			}
			return QuickAddSubmitMsg{Task: cloneNewTask(task), Revision: revision, Source: input, Review: true}
		}
	case "up", "ctrl+p":
		q.focusReviewField(q.ReviewField - 1)
		return nil
	case "down", "ctrl+n":
		q.focusReviewField(q.ReviewField + 1)
		return nil
	case "pageup", "ctrl+u":
		q.ReviewDetailScroll = max(0, q.ReviewDetailScroll-1)
		return nil
	case "pagedown", "ctrl+d":
		q.ReviewDetailScroll++
		return nil
	}
	before := q.ReviewInputs[q.ReviewField].Value()
	var cmd tea.Cmd
	q.ReviewInputs[q.ReviewField], cmd = q.ReviewInputs[q.ReviewField].Update(msg)
	if before != q.ReviewInputs[q.ReviewField].Value() {
		q.ReviewTouched[q.ReviewField] = true
		q.ReviewDetailScroll = 0
		q.CaptureRevision++
	}
	q.ParseErr = nil
	q.ensureReviewFieldVisible()
	return cmd
}

func (q *QuickAddModel) focusReviewField(index int) {
	if index < 0 {
		index = quickAddReviewFieldCount - 1
	}
	if index >= quickAddReviewFieldCount {
		index = 0
	}
	for field := range q.ReviewInputs {
		if field == index {
			continue
		}
		q.ReviewInputs[field].Blur()
	}
	q.ReviewField = index
	q.ensureReviewFieldVisible()
	q.ReviewInputs[index].Focus()
}

func (q *QuickAddModel) ensureReviewFieldVisible() {
	budget := q.reviewFieldBudget()
	if budget < 1 {
		budget = 1
	}
	maxStart := quickAddReviewFieldCount - budget
	if maxStart < 0 {
		maxStart = 0
	}
	if q.ReviewScroll > maxStart {
		q.ReviewScroll = maxStart
	}
	if q.ReviewScroll < 0 {
		q.ReviewScroll = 0
	}
	if q.ReviewField < q.ReviewScroll {
		q.ReviewScroll = q.ReviewField
	}
	if q.ReviewField >= q.ReviewScroll+budget {
		q.ReviewScroll = q.ReviewField - budget + 1
	}
	if q.ReviewScroll > maxStart {
		q.ReviewScroll = maxStart
	}
}

func (q QuickAddModel) reviewFieldBudget() int {
	height := ModalMaxRows(q.Height)
	budget := height // the frame border carries the title and keys
	if q.ParseErr != nil || q.hasBlockingReviewDiagnostic() {
		budget--
	}
	if height >= 10 {
		budget -= 2 // frozen-date guidance and timezone
	}
	if budget < 1 {
		budget = 1
	}
	return budget
}

func (q *QuickAddModel) submitReview() tea.Cmd {
	values := q.reviewValues()
	touched := q.ReviewTouched
	decisions := q.ReviewDecisions
	review := cloneInterpretation(q.Review)
	revision := q.CaptureRevision
	width, height := q.Width, q.Height
	return func() tea.Msg {
		if width > 0 && height > 0 && !reviewConfirmationAccessible(width, height) {
			return QuickAddErrorMsg{Err: fmt.Errorf("review is too small to confirm safely; enlarge the terminal or use explicit syntax only"), Revision: revision, Source: review.Source}
		}
		if err := validateReviewValues(review, values, touched, decisions); err != nil {
			return QuickAddErrorMsg{Err: err, Revision: revision, Source: review.Source}
		}
		if strings.TrimSpace(values[quickAddReviewDescription]) == "" {
			return QuickAddErrorMsg{Err: fmt.Errorf("task description cannot be empty"), Revision: revision, Source: review.Source}
		}
		if estimateValue := strings.TrimSpace(values[quickAddReviewEstimate]); estimateValue != "" {
			if _, err := domain.ParseEstimate(estimateValue); err != nil {
				return QuickAddErrorMsg{Err: err, Revision: revision, Source: review.Source}
			}
		}
		switch normalizeReviewPriority(values[quickAddReviewPriority]) {
		case "", "H", "M", "L":
		default:
			return QuickAddErrorMsg{Err: fmt.Errorf("priority must be H, M, L, or empty"), Revision: revision, Source: review.Source}
		}
		task := applyReviewValues(review.Task, values)
		if task.Recurrence != "" {
			recurrence, err := domain.ParseRecurrence(task.Recurrence)
			if err != nil {
				return QuickAddErrorMsg{Err: err, Revision: revision, Source: review.Source}
			}
			task.Recurrence = recurrence
			if task.Due == "" {
				return QuickAddErrorMsg{Err: fmt.Errorf("recurring tasks need a first due date"), Revision: revision, Source: review.Source}
			}
		}
		return QuickAddSubmitMsg{Task: cloneNewTask(task), Revision: revision, Source: review.Source, Review: true}
	}
}

// reviewValidForValues is retained as a small test/embedding seam. It uses
// current edited values and explicit clear/touch decisions, never the original
// presence of a candidate alone.
func (q QuickAddModel) reviewValidForValues(values []string) bool {
	return validateReviewValues(q.Review, values, q.ReviewTouched, q.ReviewDecisions) == nil
}

func validateReviewValues(review quickadd.Interpretation, values []string, touched [quickAddReviewFieldCount]bool, decisionArgs ...[quickAddReviewFieldCount]bool) error {
	if len(values) < quickAddReviewFieldCount {
		return fmt.Errorf("review fields are incomplete")
	}
	original := reviewTaskValues(review.Task)
	decisions := [quickAddReviewFieldCount]bool{}
	if len(decisionArgs) > 0 {
		decisions = decisionArgs[0]
	}
	semanticChanged := func(index int) bool {
		if index == quickAddReviewRecurrence {
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
				for _, candidateIndex := range reviewFieldIndexes(candidate.Field) {
					if candidateIndex == index && candidateAllowsExplicitClear(candidate) {
						return true
					}
				}
			}
		}
		return false
	}
	candidateResolved := func(candidate quickadd.Candidate, index int) bool {
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
		if candidate.Conflict && (candidate.Field == quickadd.FieldDue || candidate.Field == quickadd.FieldTime) {
			// The original recurrence anchor is already compatible with its
			// recurrence. The user must change the anchor semantically, change the
			// recurrence semantics, explicitly clear a field, or press Ctrl+R to
			// keep the candidate literal. Typing and deleting back to the same
			// value is not resolution.
			if decisions[quickAddReviewDue] || decisions[quickAddReviewRecurrence] || recurrenceValueChanged(original[quickAddReviewRecurrence], values[quickAddReviewRecurrence]) || explicitClear(quickAddReviewDue) {
				continue
			}
			if !semanticChanged(quickAddReviewDue) || !reviewedRecurrenceAnchorMatches(review, values[quickAddReviewDue], reference) {
				return fmt.Errorf("resolve the highlighted interpretation or choose explicit syntax only")
			}
			continue
		}
		for _, index := range reviewFieldIndexes(candidate.Field) {
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
			for index := range values[:quickAddReviewFieldCount] {
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
		for index := range values[:quickAddReviewFieldCount] {
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

func reviewFieldIndexes(field quickadd.CandidateField) []int {
	switch field {
	case quickadd.FieldDue, quickadd.FieldTime:
		return []int{quickAddReviewDue}
	case quickadd.FieldRecurrence:
		return []int{quickAddReviewRecurrence}
	case quickadd.FieldEstimate:
		return []int{quickAddReviewEstimate}
	case quickadd.FieldPriority:
		return []int{quickAddReviewPriority}
	case quickadd.FieldProject:
		return []int{quickAddReviewProject}
	case quickadd.FieldScheduled:
		return []int{quickAddReviewScheduled}
	case quickadd.FieldTag:
		return []int{quickAddReviewTags}
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

func candidateAllowsExplicitClear(candidate quickadd.Candidate) bool {
	text := strings.ToLower(candidate.Diagnostic)
	for _, marker := range []string{"invalid", "does not exist", "ambiguous", "incomplete", "needs a supported"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func reviewedRecurrenceAnchorMatches(review quickadd.Interpretation, value string, reference time.Time) bool {
	original, ok := reviewAnchor(review.Task.Due, reference)
	if !ok {
		return false
	}
	current, ok := reviewAnchor(value, reference)
	return ok && current.Weekday() == original.Weekday()
}

func diagnosticFieldIndexes(message string) []int {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "estimate"), strings.Contains(text, "effort"):
		return []int{quickAddReviewEstimate}
	case strings.Contains(text, "priority"):
		return []int{quickAddReviewPriority}
	case strings.Contains(text, "recurrence"), strings.Contains(text, "anchor"):
		return []int{quickAddReviewRecurrence, quickAddReviewDue}
	case strings.Contains(text, "date"), strings.Contains(text, "time"), strings.Contains(text, "due"):
		return []int{quickAddReviewDue}
	case strings.Contains(text, "description"):
		return []int{quickAddReviewDescription}
	default:
		return nil
	}
}

func reviewTaskValues(task domain.NewTask) []string {
	estimate := ""
	if task.Estimate != nil {
		estimate = task.Estimate.String()
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

func (q QuickAddModel) reviewValues() []string {
	values := make([]string, len(q.ReviewInputs))
	for index := range q.ReviewInputs {
		values[index] = strings.TrimSpace(q.ReviewInputs[index].Value())
	}
	return values
}

func applyReviewValues(task domain.NewTask, values []string) domain.NewTask {
	task = cloneNewTask(task)
	task.Description = strings.TrimSpace(values[quickAddReviewDescription])
	task.Due = strings.TrimSpace(values[quickAddReviewDue])
	task.Recurrence = strings.TrimSpace(values[quickAddReviewRecurrence])
	task.Estimate = nil
	if value := strings.TrimSpace(values[quickAddReviewEstimate]); value != "" {
		if estimate, err := domain.ParseEstimate(value); err == nil {
			task.Estimate = &estimate
		}
	}
	task.Priority = normalizeReviewPriority(values[quickAddReviewPriority])
	task.Project = strings.TrimSpace(values[quickAddReviewProject])
	task.Scheduled = strings.TrimSpace(values[quickAddReviewScheduled])
	task.Tags = splitReviewTags(values[quickAddReviewTags])
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

func reviewAnchor(value string, now time.Time) (time.Time, bool) {
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

func (q QuickAddModel) reviewProvenance(index int) string {
	var explicit, inferred bool
	for _, candidate := range q.Review.Candidates {
		matches := false
		for _, field := range reviewFieldIndexes(candidate.Field) {
			if field == index {
				matches = true
				break
			}
		}
		if !matches {
			continue
		}
		if candidate.Provenance == quickadd.ProvenanceExplicit {
			explicit = true
		}
		if candidate.Provenance == quickadd.ProvenanceInferred {
			inferred = true
		}
	}
	parts := make([]string, 0, 2)
	if explicit {
		parts = append(parts, "explicit")
	}
	if inferred {
		parts = append(parts, "inferred")
	}
	if len(parts) == 0 {
		if index == quickAddReviewDescription {
			return "remaining prose"
		}
		return "literal"
	}
	return strings.Join(parts, "+")
}

func compactProvenance(value string) string {
	text := strings.ToLower(value)
	switch {
	case strings.Contains(text, "explicit") && strings.Contains(text, "inferred"):
		return "*"
	case strings.Contains(text, "explicit"):
		return "e"
	case strings.Contains(text, "inferred"):
		return "i"
	case strings.Contains(text, "conflict"):
		return "!"
	case strings.Contains(text, "remaining"):
		return "r"
	default:
		return "l"
	}
}

func reviewDisplayValue(value string, index int, now time.Time) string {
	if index != quickAddReviewDue {
		return value
	}
	parsed, err := time.ParseInLocation("20060102T150405", strings.TrimSpace(value), now.Location())
	if err != nil {
		return value
	}
	return parsed.Format("2006-01-02 15:04 MST")
}

func (q QuickAddModel) hasBlockingReviewDiagnostic() bool {
	for _, diagnostic := range q.Review.Diagnostics {
		if diagnostic.Blocking {
			return true
		}
	}
	for _, candidate := range q.Review.Candidates {
		if candidate.Blocking {
			return true
		}
	}
	return false
}

func (q QuickAddModel) reviewDetailRows(width int) []FrameRow {
	rows := make([]FrameRow, 0)
	appendWrapped := func(text string, tone Tone) {
		for _, line := range WrapText(text, width) {
			rows = append(rows, row(sp(line, tone)))
		}
	}
	if q.ParseErr != nil {
		appendWrapped(q.ParseErr.Error(), ToneRed)
		return rows
	}
	for _, diagnostic := range q.Review.Diagnostics {
		tone := ToneMuted
		if diagnostic.Blocking {
			tone = ToneRed
		}
		appendWrapped(diagnostic.Message, tone)
	}
	if recurrence := strings.TrimSpace(q.ReviewInputs[quickAddReviewRecurrence].Value()); recurrence != "" {
		reference := q.Review.ReferenceTime
		if reference.IsZero() {
			reference = q.currentTime()
		}
		if anchor, ok := reviewAnchor(q.ReviewInputs[quickAddReviewDue].Value(), reference); ok {
			if next, ok := domain.RecurrenceNext(anchor, recurrence); ok {
				appendWrapped("Next occurrence: "+next.Format("2006-01-02 15:04 MST"), ToneMuted)
			}
		}
	}
	for _, candidate := range q.Review.Candidates {
		status := "inferred"
		if candidate.Provenance == quickadd.ProvenanceExplicit {
			status = "explicit"
		} else if candidate.Conflict {
			status = "conflict"
		} else if candidate.Accepted {
			status = "accepted"
		}
		for _, index := range reviewFieldIndexes(candidate.Field) {
			if q.ReviewDecisions[index] {
				status = "literal"
				break
			}
		}
		line := fmt.Sprintf("%s: %s (%s)", candidate.Field, candidate.Text, status)
		if candidate.Diagnostic != "" {
			line += " · " + candidate.Diagnostic
		}
		tone := ToneMuted
		resolved := false
		for _, index := range reviewFieldIndexes(candidate.Field) {
			if q.ReviewDecisions[index] {
				resolved = true
				break
			}
		}
		if candidate.Blocking && !resolved {
			tone = ToneRed
		}
		appendWrapped(line, tone)
	}
	return rows
}

// reviewFieldAffixes is shared by sizing and rendering so the input viewport
// cannot extend behind a label, provenance marker, or formatted date.
func (q QuickAddModel) reviewFieldAffixes(index int) (prefix, suffix string) {
	provenance := q.reviewProvenance(index)
	if q.compactReview() {
		return quickAddReviewFieldNames[index] + compactProvenance(provenance) + " ", ""
	}
	prefix = fmt.Sprintf("%-11s %-10s ", quickAddReviewFieldNames[index], "["+provenance+"]")
	if index == quickAddReviewDue {
		reference := q.Review.ReferenceTime
		if reference.IsZero() {
			reference = q.currentTime()
		}
		raw := strings.TrimSpace(q.ReviewInputs[index].Value())
		if display := reviewDisplayValue(raw, index, reference); display != raw && display != "" {
			prefix += display + " ["
			suffix = "]"
		}
	}
	return prefix, suffix
}

func (q QuickAddModel) compactReview() bool {
	return quickAddContentWidth(q.Width) < 60 || ModalMaxRows(q.Height) < 10
}

// quickAddContentWidth is the content column of the capture frame.
func quickAddContentWidth(terminalWidth int) int {
	return FrameContentWidth(ModalWidth(QuickAddWidth, terminalWidth))
}

func (q QuickAddModel) reviewView() string {
	icons := q.Icons.orUnicode()
	frameWidth := ModalWidth(QuickAddWidth, q.Width)
	width := FrameContentWidth(frameWidth)
	height := ModalMaxRows(q.Height)

	compact := q.compactReview()
	info := make([]FrameRow, 0, 2)
	if !compact && height >= 10 {
		info = append(info, row(muted(Truncate("Inferred values are frozen until you go back and reinterpret", width))))
		if q.Review.Timezone != "" {
			info = append(info, row(muted(Truncate("Timezone: "+q.Review.Timezone, width))))
		}
	}

	details := q.reviewDetailRows(width)
	if compact && q.ParseErr == nil && !q.hasBlockingReviewDiagnostic() && len(q.Review.Diagnostics) == 0 {
		// Compact rows carry provenance markers themselves; spend the scarce
		// height on the focused value rather than hiding it behind candidates.
		details = nil
	}
	available := max(1, height-len(info))
	// Keep at least one field visible. Blocking diagnostics take priority over
	// candidate prose when a short terminal cannot show everything at once.
	detailBudget := min(len(details), max(0, available-1))
	fieldBudget := available - detailBudget
	if fieldBudget < 1 {
		fieldBudget = 1
		detailBudget = max(0, available-fieldBudget)
	}
	if detailBudget < len(details) {
		detailStart := min(max(0, q.ReviewDetailScroll), len(details)-detailBudget)
		details = details[detailStart : detailStart+detailBudget]
	}
	fieldStart, fieldEnd := q.reviewFieldWindow(fieldBudget)

	rows := append(info, details...)
	for index := fieldStart; index < fieldEnd; index++ {
		rows = append(rows, q.reviewFieldRow(index, width, icons))
	}
	frame := Frame{
		Title: "Review capture", Rows: rows, Width: frameWidth, MaxRows: height,
		Keys: []Hint{{"ctrl+s", "confirm"}, {"esc", "back"}, {"tab", "field"}, {"ctrl+r", "resolve"}, {"ctrl+x", "explicit only"}, {"pgup/dn", "details"}},
	}
	return strings.Join(q.Styles.RenderFrame(frame, icons), "\n")
}

// reviewFieldRow budgets the input by the same affixes it renders, so a long
// value scrolls to keep its cursor visible instead of clipping behind them.
func (q QuickAddModel) reviewFieldRow(index, width int, icons Icons) FrameRow {
	prefix, suffix := q.reviewFieldAffixes(index)
	input := q.ReviewInputs[index]
	focused := index == q.ReviewField
	value := inputSpans(input.Value(), input.Position(), width-lipgloss.Width(prefix)-lipgloss.Width(suffix), focused, input.Placeholder, nil)
	if !focused {
		return row(append(append([]Span{muted(prefix)}, value...), muted(suffix))...)
	}
	spans := append(append([]Span{txt(prefix).bold()}, value...), txt(suffix))
	return FrameRow{Spans: spans, Mark: icons.Selection, Bg: FillSelection}
}

func (q QuickAddModel) reviewFieldWindow(budget int) (int, int) {
	if budget < 1 {
		budget = 1
	}
	if budget >= quickAddReviewFieldCount {
		return 0, quickAddReviewFieldCount
	}
	start := q.ReviewScroll
	if start < 0 {
		start = 0
	}
	if start > q.ReviewField {
		start = q.ReviewField
	}
	if q.ReviewField >= start+budget {
		start = q.ReviewField - budget + 1
	}
	if maxStart := quickAddReviewFieldCount - budget; start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	return start, min(quickAddReviewFieldCount, start+budget)
}

func reviewConfirmationAccessible(width, height int) bool {
	return width >= MinimumWidth && height >= MinimumHeight
}

func cloneInterpretation(value quickadd.Interpretation) quickadd.Interpretation {
	result := value
	result.Task = cloneNewTask(value.Task)
	result.ExplicitOnly = cloneNewTask(value.ExplicitOnly)
	result.Candidates = append([]quickadd.Candidate(nil), value.Candidates...)
	result.Diagnostics = append([]quickadd.Diagnostic(nil), value.Diagnostics...)
	return result
}

func cloneNewTask(value domain.NewTask) domain.NewTask {
	result := value
	result.Tags = append([]string(nil), value.Tags...)
	if value.Estimate != nil {
		result.Estimate = &domain.Estimate{Minutes: value.Estimate.Minutes}
	}
	return result
}

// RestoreFailedTask reopens the last confirmed draft after an adapter/hook
// failure. It never emits a retry command by itself.
func (q *QuickAddModel) RestoreFailedTask(task domain.NewTask, source string, reviewed bool, err error) {
	q.CaptureRevision++
	q.Open = true
	q.OriginalInput = source
	q.ParseErr = err
	q.Input.SetValue(source)
	q.Input.CursorEnd()
	q.SuggestionsOpen = false
	q.Suggestions = nil
	if !reviewed {
		q.ReviewOpen = false
		q.Input.Focus()
		return
	}
	q.ReviewOpen = true
	q.Review = cloneInterpretation(q.Review)
	q.Review.Source = source
	q.Review.Task = cloneNewTask(task)
	q.Review.ExplicitOnly = cloneNewTask(task)
	q.Review.Valid = true
	q.Review.RequiresReview = true
	q.Review.Diagnostics = nil
	for index := range q.Review.Candidates {
		if q.Review.Candidates[index].Blocking {
			q.Review.Candidates[index].Blocking = false
			q.Review.Candidates[index].Accepted = true
			q.Review.Candidates[index].Diagnostic = ""
		}
	}
	if q.Review.ReferenceTime.IsZero() {
		q.Review.ReferenceTime = q.currentTime()
		q.Review.Timezone = q.Review.ReferenceTime.Location().String()
	}
	q.ReviewRevision = q.CaptureRevision
	q.ReviewTouched = [quickAddReviewFieldCount]bool{}
	q.ReviewDecisions = [quickAddReviewFieldCount]bool{}
	q.ReviewField = quickAddReviewDescription
	q.ReviewScroll = 0
	q.ReviewDetailScroll = 0
	q.setReviewInputValues(task)
	q.focusReviewField(quickAddReviewDescription)
}
