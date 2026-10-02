package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
	"github.com/jvrviegas/momentum-task-manager/internal/quickadd"
)

// QuickAddSubmitMsg is emitted after a successful parse; it contains no
// process execution and can be routed to the app command layer. Revision ties
// the immutable task to the capture that produced it.
type QuickAddSubmitMsg struct {
	Task     domain.NewTask
	Revision uint64
	Source   string
	Review   bool
}

// QuickAddReviewMsg asks the app to keep the capture overlay open while the
// user reviews inferred metadata.
type QuickAddReviewMsg struct {
	Interpretation quickadd.Interpretation
	Revision       uint64
}

// QuickAddModel owns the centered capture modal and its contextual suggestions.
type QuickAddModel struct {
	Input                 textinput.Model
	Suggestions           []quickadd.Suggestion
	SuggestionIndex       int
	SuggestionsOpen       bool
	Open                  bool
	ParseErr              error
	Projects              []string
	ProjectLabels         map[string]string
	Tags                  []string
	Width                 int
	Height                int
	Now                   time.Time
	NowProvider           func() time.Time
	OriginalInput         string
	Literals              []quickadd.Span
	Review                quickadd.Interpretation
	ReviewOpen            bool
	ReviewField           int
	ReviewScroll          int
	ReviewDetailScroll    int
	ReviewTouched         [quickAddReviewFieldCount]bool
	ReviewDecisions       [quickAddReviewFieldCount]bool
	ReviewInputs          [quickAddReviewFieldCount]textinput.Model
	CaptureRevision       uint64
	ReviewRevision        uint64
	LastSubmittedRevision uint64
	LastSubmittedSource   string
	LastSubmittedReview   bool
	Styles                Styles
	Icons                 Icons
}

// NewQuickAdd creates a focused quick-capture model.
func NewQuickAdd(styles Styles, icons Icons) QuickAddModel {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "What needs doing?"
	model := QuickAddModel{Input: input, Styles: styles, Icons: icons, Now: time.Now()}
	for index := range model.ReviewInputs {
		model.ReviewInputs[index] = textinput.New()
		model.ReviewInputs[index].Prompt = ""
	}
	return model
}

// OpenQuickAdd resets parse state and focuses the command bar.
func (q *QuickAddModel) OpenQuickAdd(value string) tea.Cmd {
	q.CaptureRevision++
	q.Open = true
	q.ReviewOpen = false
	q.Review = quickadd.Interpretation{}
	q.ReviewField = 0
	q.ReviewScroll = 0
	q.ReviewDetailScroll = 0
	q.ReviewTouched = [quickAddReviewFieldCount]bool{}
	q.ReviewDecisions = [quickAddReviewFieldCount]bool{}
	q.OriginalInput = value
	q.Literals = nil
	q.ParseErr = nil
	q.Input.SetValue(value)
	q.Input.CursorEnd()
	q.refreshSuggestions()
	return q.Input.Focus()
}

// Close closes the bar without touching the underlying Taskwarrior client.
func (q *QuickAddModel) Close() {
	// Closing invalidates every command that was scheduled for this capture.
	q.CaptureRevision++
	q.Open = false
	q.ReviewOpen = false
	q.SuggestionsOpen = false
	q.Suggestions = nil
	q.Input.Blur()
	for index := range q.ReviewInputs {
		q.ReviewInputs[index].Blur()
	}
}

func (q *QuickAddModel) SetSize(width, height int) {
	q.Width, q.Height = width, height
	q.ensureReviewFieldVisible()
}

func (q *QuickAddModel) SetCatalog(projects, tags []string) {
	q.Projects = append([]string(nil), projects...)
	q.Tags = append([]string(nil), tags...)
	q.refreshSuggestions()
}

// SetProjectCatalog installs an owned snapshot of project values and labels.
func (q *QuickAddModel) SetProjectCatalog(projects domain.ProjectCatalog) {
	q.ProjectLabels = projects.Labels()
	q.SetCatalog(projects.Values(), q.Tags)
}

// Update routes overlay-owned keys and delegates ordinary editing to Bubbles.
func (q *QuickAddModel) Update(msg tea.Msg) (*QuickAddModel, tea.Cmd) {
	if !q.Open {
		return q, nil
	}
	if q.ReviewOpen {
		return q, q.updateReview(msg)
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc", "escape":
			if q.SuggestionsOpen {
				q.SuggestionsOpen = false
				return q, nil
			}
			if span, ok := q.phraseAtCursor(); ok {
				q.Literals = append(q.Literals, span)
				q.CaptureRevision++
				return q, nil
			}
			q.Close()
			return q, nil
		case "tab":
			if q.SuggestionsOpen && len(q.Suggestions) > 0 {
				q.acceptSuggestion()
				q.CaptureRevision++
				return q, nil
			}
		case "up", "ctrl+p":
			if q.SuggestionsOpen {
				q.moveSuggestion(-1)
				return q, nil
			}
		case "down", "ctrl+n":
			if q.SuggestionsOpen {
				q.moveSuggestion(1)
				return q, nil
			}
		case "enter":
			return q, q.submit()
		case "ctrl+k":
			// Ctrl+K is the global opener; inside the bar it must not delete
			// input through textinput's default key map.
			return q, nil
		}
	}
	before := q.Input.Value()
	var cmd tea.Cmd
	q.Input, cmd = q.Input.Update(msg)
	if before != q.Input.Value() {
		q.CaptureRevision++
		q.Literals = shiftLiterals(before, q.Input.Value(), q.Literals)
	}
	q.ParseErr = nil
	q.refreshSuggestions()
	return q, cmd
}

func (q *QuickAddModel) refreshSuggestions() {
	if !q.Open {
		q.Suggestions = nil
		q.SuggestionsOpen = false
		return
	}
	ctx := quickadd.ContextAt(q.Input.Value(), q.Input.Position())
	q.Suggestions = quickadd.SuggestionsWithProjectLabels(ctx, q.Projects, q.Tags, q.ProjectLabels, q.currentTime())
	q.SuggestionsOpen = len(q.Suggestions) > 0
	if q.SuggestionIndex >= len(q.Suggestions) {
		q.SuggestionIndex = 0
	}
}

func (q *QuickAddModel) currentTime() time.Time {
	if q.NowProvider != nil {
		if value := q.NowProvider(); !value.IsZero() {
			return value
		}
	}
	if q.Now.IsZero() {
		return time.Now()
	}
	return q.Now
}

func (q *QuickAddModel) moveSuggestion(delta int) {
	if len(q.Suggestions) == 0 {
		return
	}
	q.SuggestionIndex = (q.SuggestionIndex + delta) % len(q.Suggestions)
	if q.SuggestionIndex < 0 {
		q.SuggestionIndex += len(q.Suggestions)
	}
}

func (q *QuickAddModel) acceptSuggestion() {
	if len(q.Suggestions) == 0 {
		return
	}
	before := q.Input.Value()
	ctx := quickadd.ContextAt(before, q.Input.Position())
	value, cursor := quickadd.ApplySuggestion(before, ctx, q.Suggestions[q.SuggestionIndex])
	q.Literals = shiftLiterals(before, value, q.Literals)
	q.Input.SetValue(value)
	q.Input.SetCursor(cursor)
	q.CaptureRevision++
	q.refreshSuggestions()
}

func (q *QuickAddModel) submit() tea.Cmd {
	input := q.Input.Value()
	now := q.currentTime()
	revision := q.CaptureRevision
	literals := append([]quickadd.Span(nil), q.Literals...)
	return func() tea.Msg {
		interpretation, err := quickadd.InterpretWithLiterals(input, now, literals)
		if err != nil {
			return QuickAddErrorMsg{Err: err, Revision: revision, Source: input}
		}
		if !interpretation.Valid {
			return QuickAddReviewMsg{Interpretation: interpretation, Revision: revision}
		}
		return QuickAddSubmitMsg{Task: cloneNewTask(interpretation.Task), Revision: revision, Source: input}
	}
}

// QuickAddErrorMsg keeps the input visible while reporting a parse failure.
type QuickAddErrorMsg struct {
	Err      error
	Revision uint64
	Source   string
}

// ApplyMessage consumes the typed parse result and keeps errors local to the
// overlay. A successful message is returned for the app layer to route.
func (q *QuickAddModel) ApplyMessage(msg tea.Msg) (tea.Msg, bool) {
	switch message := msg.(type) {
	case QuickAddErrorMsg:
		if !q.currentRevision(message.Revision) || !q.currentSource(message.Source) {
			return nil, false
		}
		q.ParseErr = message.Err
		return nil, true
	case QuickAddReviewMsg:
		if !q.currentRevision(message.Revision) || message.Interpretation.Source != q.Input.Value() {
			return nil, false
		}
		q.openReview(message.Interpretation)
		return nil, true
	case QuickAddSubmitMsg:
		if !q.currentRevision(message.Revision) || !q.currentSource(message.Source) {
			return nil, false
		}
		q.LastSubmittedRevision = message.Revision
		q.LastSubmittedSource = message.Source
		q.LastSubmittedReview = message.Review
		q.Close()
		return message, true
	default:
		return nil, false
	}
}

// View renders the capture frame: the input with parsed tokens underlined in
// their field color, then either the trigger guide or, while a trigger is
// active, the suggestion list that replaces it.
func (q QuickAddModel) View() string {
	if !q.Open || q.Width <= 0 || q.Height <= 0 {
		return ""
	}
	if q.ReviewOpen {
		return q.reviewView()
	}
	icons := q.Icons.orUnicode()
	width := ModalWidth(QuickAddWidth, q.Width)
	content := FrameContentWidth(width)
	narrow := q.Width < NarrowBreakpoint
	maxRows := ModalMaxRows(q.Height)
	if q.Height < 3 {
		content = width - 1
	}
	input := append([]Span{sp(icons.Prompt, ToneAccent), txt(" ")},
		inputSpans(q.Input.Value(), q.Input.Position(), content-2, true, q.Input.Placeholder, captureTokenCell(q.Input.Value(), q.currentTime(), q.Literals))...)
	if q.Height < 3 {
		return q.Styles.Line(width, FillPanel, append([]Span{txt(" ")}, input...)...)
	}
	rows := []FrameRow{{}, row(input...)}
	keys := []Hint{{"enter", "add"}, {"esc", "close"}}
	if _, ok := q.phraseAtCursor(); ok {
		keys = []Hint{{"enter", "add"}, {"esc", "keep as text"}}
	}
	switch {
	case q.ParseErr != nil:
		rows = append(rows, FrameRow{})
		for _, line := range WrapText(q.ParseErr.Error(), max(1, content-2)) {
			rows = append(rows, row(sp(icons.Error, ToneRed).bold(), txt(" "), sp(line, ToneRed)))
		}
	case q.SuggestionsOpen && len(q.Suggestions) > 0:
		keys = []Hint{{"↑↓", "select"}, {"tab", "accept"}, {"esc", "close"}}
		rows = append(rows, q.suggestionRows(content, narrow, maxRows-len(rows), icons)...)
	default:
		rows = append(rows, captureGuide(content, maxRows-len(rows), icons)...)
	}
	rows = append(rows, FrameRow{})
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	frame := Frame{
		Title: "Quick capture", Context: []Span{muted("ctrl+k")}, Rows: rows, Width: width, MaxRows: maxRows, Keys: keys,
	}
	return strings.Join(q.Styles.RenderFrame(frame, icons), "\n")
}

// captureTokenCell colors trigger tokens (#project, !priority, @due,
// >scheduled, +tag, ~estimate, ^recurrence) by field and underlines them, so
// parsing is visible before Enter. A leading backslash escapes a trigger.
// Natural-language phrases the interpreter accepts are underlined the same
// way, and phrases that would block the capture are underlined in red.
// Literals are phrases the user chose to keep as text.
func captureTokenCell(value string, now time.Time, literals []quickadd.Span) func(int, rune) Span {
	runes := []rune(value)
	tones := make([]Tone, len(runes))
	parsed := make([]bool, len(runes))
	if interpretation, err := quickadd.InterpretWithLiterals(value, now, literals); err == nil {
		for _, candidate := range interpretation.Candidates {
			if !highlightedCandidate(candidate) {
				continue
			}
			tone := candidateTone(candidate.Field)
			if candidate.Blocking {
				tone = ToneRed
			}
			for index := max(0, candidate.Start); index < min(candidate.End, len(runes)); index++ {
				tones[index], parsed[index] = tone, true
			}
		}
	}
	for start := 0; start < len(runes); {
		if unicode.IsSpace(runes[start]) {
			start++
			continue
		}
		end := start
		for end < len(runes) && !unicode.IsSpace(runes[end]) {
			end++
		}
		tone, ok := triggerTone(runes[start])
		for index := start; index < end && ok; index++ {
			tones[index], parsed[index] = tone, true
		}
		start = end
	}
	return func(index int, r rune) Span {
		if index < len(parsed) && parsed[index] {
			return Span{Text: string(r), Tone: tones[index], Underline: true}
		}
		return txt(string(r))
	}
}

func highlightedCandidate(candidate quickadd.Candidate) bool {
	return candidate.Provenance == quickadd.ProvenanceInferred && (candidate.Accepted || candidate.Blocking)
}

// phraseAtCursor returns the highlighted natural-language phrase the cursor is
// on or directly after, which Esc keeps as text.
func (q QuickAddModel) phraseAtCursor() (quickadd.Span, bool) {
	interpretation, err := quickadd.InterpretWithLiterals(q.Input.Value(), q.currentTime(), q.Literals)
	if err != nil {
		return quickadd.Span{}, false
	}
	cursor := q.Input.Position()
	for _, candidate := range interpretation.Candidates {
		if highlightedCandidate(candidate) && candidate.Start <= cursor && cursor <= candidate.End {
			return quickadd.Span{Start: candidate.Start, End: candidate.End}, true
		}
	}
	return quickadd.Span{}, false
}

// shiftLiterals moves kept phrases with an edit between before and after.
// A kept phrase the edit touches is dropped so the new text is recognized
// again.
func shiftLiterals(before, after string, literals []quickadd.Span) []quickadd.Span {
	if len(literals) == 0 {
		return nil
	}
	old, next := []rune(before), []rune(after)
	prefix := 0
	for prefix < len(old) && prefix < len(next) && old[prefix] == next[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(old)-prefix && suffix < len(next)-prefix && old[len(old)-1-suffix] == next[len(next)-1-suffix] {
		suffix++
	}
	editEnd, delta := len(old)-suffix, len(next)-len(old)
	var kept []quickadd.Span
	for _, literal := range literals {
		switch {
		case literal.End <= prefix:
			kept = append(kept, literal)
		case literal.Start >= editEnd:
			kept = append(kept, quickadd.Span{Start: literal.Start + delta, End: literal.End + delta})
		}
	}
	return kept
}

// candidateTone matches an inferred phrase to its explicit trigger's color.
func candidateTone(field quickadd.CandidateField) Tone {
	switch field {
	case quickadd.FieldProject:
		return ToneCyan
	case quickadd.FieldPriority:
		return ToneHigh
	case quickadd.FieldDue, quickadd.FieldTime:
		return ToneAccent
	case quickadd.FieldScheduled:
		return ToneMedium
	case quickadd.FieldTag:
		return ToneGreen
	case quickadd.FieldEstimate:
		return TonePurple
	case quickadd.FieldRecurrence:
		return ToneTeal
	default:
		return ToneText
	}
}

func triggerTone(trigger rune) (Tone, bool) {
	switch trigger {
	case '#':
		return ToneCyan, true
	case '!':
		return ToneHigh, true
	case '@':
		return ToneAccent, true
	case '>':
		return ToneMedium, true
	case '+':
		return ToneGreen, true
	case '~':
		return TonePurple, true
	case '^':
		return ToneTeal, true
	default:
		return ToneText, false
	}
}

func captureGuide(width, budget int, icons Icons) []FrameRow {
	if budget < 2 {
		return nil
	}
	type trigger struct {
		sigil, field, example string
	}
	triggers := []trigger{
		{"#", "project", "#work.client"}, {"+", "tag", "+planning"},
		{"!", "priority", "!high  !low"}, {"@", "due", "@tomorrow"},
		{">", "scheduled", ">monday"}, {"~", "estimate", "~1h30m"},
	}
	cell := func(t trigger, exampleWidth int) []Span {
		tone, _ := triggerTone([]rune(t.sigil)[0])
		spans := []Span{sp(t.sigil, tone).bold(), txt(" "), muted(fmt.Sprintf("%-10s", t.field))}
		if exampleWidth > 0 {
			spans = append(spans, txt(fmt.Sprintf("%-*s", exampleWidth, Truncate(t.example, exampleWidth))))
		}
		return spans
	}
	perRow, exampleWidth := 2, 22
	if width < 70 {
		perRow, exampleWidth = 1, min(22, width-12)
	}
	if exampleWidth < 6 {
		exampleWidth = 0
	}
	var triggerRows []FrameRow
	for index := 0; index < len(triggers); index += perRow {
		spans := cell(triggers[index], exampleWidth)
		if perRow == 2 && index+1 < len(triggers) {
			spans = append(append(spans, gap(2)), cell(triggers[index+1], exampleWidth)...)
		}
		triggerRows = append(triggerRows, row(spans...))
	}
	// Every trigger row survives first; decoration fills what is left.
	room := budget - 1 - len(triggerRows)
	if room < 0 {
		// A partial guide misleads; show every trigger or none.
		return nil
	}
	var head, tail []FrameRow
	if room >= 2 {
		head = []FrameRow{{}, row(muted("TRIGGERS"))}
		room -= 2
	}
	if room >= 2 {
		tail = []FrameRow{{}, row(muted(`\#launch keeps a literal "#launch" in the title.`))}
		room -= 2
	}
	if room >= 1 && len(head) > 0 {
		head = []FrameRow{{}, row(rule(icons.Rule)), row(muted("TRIGGERS"))}
	}
	return append(append(head, triggerRows...), tail...)
}

func (q QuickAddModel) suggestionRows(width int, narrow bool, budget int, icons Icons) []FrameRow {
	ctx := quickadd.ContextAt(q.Input.Value(), q.Input.Position())
	limit := 6
	if narrow {
		limit = 2
	}
	limit = min(limit, len(q.Suggestions), max(1, budget-5))
	start := max(0, q.SuggestionIndex-limit+1)
	header := []Span{muted(strings.ToUpper(suggestionHeading(q.Suggestions[0].Kind)))}
	if !narrow && ctx.Prefix != "" {
		header = append(header, gap(2), muted(fmt.Sprintf("matching %q", ctx.Prefix)))
	}
	header = append(header, grow(), muted(fmt.Sprintf("%d", len(q.Suggestions))))
	rows := []FrameRow{{}, row(header...)}
	tone := suggestionTone(q.Suggestions[0].Kind)
	for index := start; index < start+limit; index++ {
		suggestion := q.Suggestions[index]
		selected := index == q.SuggestionIndex
		spans := matchedPrefix(suggestion.Text, ctx.Token, tone, selected)
		spans = append(spans, grow())
		if suggestion.Label != "" && !narrow {
			spans = append(spans, muted(suggestion.Label))
		}
		r := row(spans...)
		if selected {
			r.Mark, r.Bg = icons.Selection, FillSelection
		}
		rows = append(rows, r)
	}
	if !narrow {
		selected := q.Suggestions[q.SuggestionIndex]
		rows = append(rows, FrameRow{}, row(txt("tab").bold(), muted(" inserts "), sp(Truncate(selected.Text, max(1, width-12)), tone)))
	}
	return rows
}

// matchedPrefix bolds and underlines the part of a suggestion already typed.
func matchedPrefix(text, typed string, tone Tone, selected bool) []Span {
	base := Span{Text: text, Tone: tone, Bold: selected}
	if typed == "" || !strings.HasPrefix(strings.ToLower(text), strings.ToLower(typed)) {
		return []Span{base}
	}
	head, tail := base, base
	head.Text, head.Bold, head.Underline = text[:len(typed)], true, true
	tail.Text = text[len(typed):]
	return []Span{head, tail}
}

func suggestionTone(kind quickadd.SuggestionKind) Tone {
	switch kind {
	case quickadd.SuggestionProject:
		return ToneCyan
	case quickadd.SuggestionPriority:
		return ToneHigh
	case quickadd.SuggestionDue:
		return ToneAccent
	case quickadd.SuggestionScheduled:
		return ToneMedium
	case quickadd.SuggestionTag:
		return ToneGreen
	case quickadd.SuggestionEstimate:
		return TonePurple
	case quickadd.SuggestionRecurrence:
		return ToneTeal
	default:
		return ToneText
	}
}

func suggestionHeading(kind quickadd.SuggestionKind) string {
	switch kind {
	case quickadd.SuggestionProject:
		return "Projects"
	case quickadd.SuggestionPriority:
		return "Priorities"
	case quickadd.SuggestionDue:
		return "Due dates"
	case quickadd.SuggestionScheduled:
		return "Scheduled dates"
	case quickadd.SuggestionTag:
		return "Tags"
	case quickadd.SuggestionEstimate:
		return "Estimates"
	case quickadd.SuggestionRecurrence:
		return "Recurrence"
	default:
		return "Suggestions"
	}
}

// ErrorText is a plain error accessor for status rendering and tests.
func (q QuickAddModel) ErrorText() string {
	if q.ParseErr == nil {
		return ""
	}
	return strings.TrimSpace(q.ParseErr.Error())
}

func (q QuickAddModel) currentRevision(revision uint64) bool {
	return q.Open && revision == q.CaptureRevision
}

func (q QuickAddModel) currentSource(source string) bool {
	return source == "" || source == q.Input.Value()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
