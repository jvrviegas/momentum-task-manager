package bridge

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
	"github.com/jvrviegas/momentum-task-manager/internal/quickadd"
)

// Timestamps are RFC 3339 in UTC. Text offsets (cursor, spans, candidate
// start/end) count Unicode scalar values, matching Go rune offsets.

type statusResult struct {
	TaskwarriorVersion     string       `json:"taskwarriorVersion"`
	SyncConfigured         bool         `json:"syncConfigured"`
	EstimateUDA            string       `json:"estimateUDA"`
	ConfigPath             string       `json:"configPath"`
	Sync                   syncSettings `json:"sync"`
	RefreshIntervalSeconds int          `json:"refreshIntervalSeconds"`
}

type syncSettings struct {
	Enabled              bool `json:"enabled"`
	Startup              bool `json:"startup"`
	Shutdown             bool `json:"shutdown"`
	IntervalSeconds      int  `json:"intervalSeconds"`
	MutationDelaySeconds int  `json:"mutationDelaySeconds"`
}

type loadResult struct {
	Tasks          []taskWire    `json:"tasks"`
	Today          []section     `json:"today"`
	Inbox          []section     `json:"inbox"`
	Completed      []section     `json:"completed"`
	CompletedError string        `json:"completedError,omitempty"`
	Context        string        `json:"context,omitempty"`
	Projects       []projectWire `json:"projects"`
}

type section struct {
	Title   string   `json:"title"`
	Project string   `json:"project,omitempty"`
	UUIDs   []string `json:"uuids"`
}

type projectWire struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
}

type taskWire struct {
	UUID             string            `json:"uuid"`
	ID               int               `json:"id"`
	Description      string            `json:"description"`
	Status           string            `json:"status"`
	Project          string            `json:"project,omitempty"`
	Priority         string            `json:"priority,omitempty"`
	EstimateMinutes  int               `json:"estimateMinutes,omitempty"`
	Estimate         string            `json:"estimate,omitempty"`
	EstimateWarning  string            `json:"estimateWarning,omitempty"`
	EstimateRaw      string            `json:"estimateRaw,omitempty"`
	Entry            string            `json:"entry,omitempty"`
	End              string            `json:"end,omitempty"`
	Modified         string            `json:"modified,omitempty"`
	Due              string            `json:"due,omitempty"`
	Scheduled        string            `json:"scheduled,omitempty"`
	Start            string            `json:"start,omitempty"`
	Wait             string            `json:"wait,omitempty"`
	Until            string            `json:"until,omitempty"`
	Tags             []string          `json:"tags"`
	Annotations      []annotationWire  `json:"annotations"`
	Depends          []string          `json:"depends"`
	Recur            string            `json:"recur,omitempty"`
	RecurDisplay     string            `json:"recurDisplay,omitempty"`
	Parent           string            `json:"parent,omitempty"`
	RecurrenceTarget string            `json:"recurrenceTarget,omitempty"`
	Urgency          float64           `json:"urgency"`
	TodayGroup       string            `json:"todayGroup,omitempty"`
	Edit             editFields        `json:"edit"`
	Extra            map[string]string `json:"extra,omitempty"`
}

type annotationWire struct {
	Description string `json:"description"`
	Entry       string `json:"entry,omitempty"`
}

// editFields are the eight editable values as text, exactly as the TUI editor
// shows them: dates as local "YYYY-MM-DD HH:MM" and tags space separated.
type editFields struct {
	Description string `json:"description"`
	Project     string `json:"project"`
	Priority    string `json:"priority"`
	Due         string `json:"due"`
	Scheduled   string `json:"scheduled"`
	Tags        string `json:"tags"`
	Estimate    string `json:"estimate"`
	Recurrence  string `json:"recurrence"`
}

type span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type interpretParams struct {
	Input    string `json:"input"`
	Literals []span `json:"literals"`
}

type suggestParams struct {
	Input  string `json:"input"`
	Cursor int    `json:"cursor"`
}

type reviewParams struct {
	Token     int64                           `json:"token"`
	Values    []string                        `json:"values"`
	Touched   [quickadd.ReviewFieldCount]bool `json:"touched"`
	Decisions [quickadd.ReviewFieldCount]bool `json:"decisions"`
}

type modifyParams struct {
	UUID   string     `json:"uuid"`
	Before editFields `json:"before"`
	After  editFields `json:"after"`
}

type uuidParams struct {
	UUID string `json:"uuid"`
}

type syncResult struct {
	Changed bool `json:"changed"`
}

type modifyResult struct {
	Changed bool `json:"changed"`
}

type captureResult struct {
	Added  bool                `json:"added"`
	Review *interpretationWire `json:"review,omitempty"`
}

type interpretationWire struct {
	Token          int64            `json:"token,omitempty"`
	Source         string           `json:"source"`
	Valid          bool             `json:"valid"`
	RequiresReview bool             `json:"requiresReview"`
	Description    string           `json:"description"`
	Candidates     []candidateWire  `json:"candidates"`
	Diagnostics    []diagnosticWire `json:"diagnostics"`
	// ReviewValues are the correction-review fields in quickadd.Review* order.
	ReviewValues []string `json:"reviewValues"`
}

type candidateWire struct {
	Field      string `json:"field"`
	Text       string `json:"text"`
	Value      string `json:"value"`
	Start      int    `json:"start"`
	End        int    `json:"end"`
	Provenance string `json:"provenance"`
	Accepted   bool   `json:"accepted"`
	Conflict   bool   `json:"conflict"`
	Blocking   bool   `json:"blocking"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

type diagnosticWire struct {
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

type suggestResult struct {
	Active      bool             `json:"active"`
	Kind        string           `json:"kind,omitempty"`
	Start       int              `json:"start"`
	End         int              `json:"end"`
	Suggestions []suggestionWire `json:"suggestions"`
}

type suggestionWire struct {
	Value string `json:"value"`
	Text  string `json:"text"`
	Label string `json:"label,omitempty"`
	// Input and Cursor are the capture text after accepting this suggestion.
	Input  string `json:"input"`
	Cursor int    `json:"cursor"`
}

// knownFields are export properties taskWire models; anything else is shown
// read-only as Extra.
var knownFields = map[string]bool{
	"uuid": true, "id": true, "description": true, "status": true, "project": true,
	"priority": true, "estimate": true, "entry": true, "end": true, "modified": true,
	"due": true, "scheduled": true, "start": true, "wait": true, "until": true,
	"tags": true, "annotations": true, "depends": true, "recur": true, "parent": true,
	"mask": true, "imask": true, "rtype": true, "urgency": true,
}

func newTaskWire(task domain.Task, now time.Time) taskWire {
	wire := taskWire{
		UUID:            task.UUID,
		ID:              task.ID,
		Description:     task.Description,
		Status:          task.Status,
		Project:         task.Project,
		Priority:        task.Priority,
		EstimateWarning: task.EstimateWarning,
		Entry:           stamp(task.Entry),
		End:             stamp(task.End),
		Modified:        stamp(task.Modified),
		Due:             stamp(task.Due),
		Scheduled:       stamp(task.Scheduled),
		Start:           stamp(task.Start),
		Wait:            stamp(task.Wait),
		Until:           stamp(task.Until),
		Tags:            append([]string{}, task.Tags...),
		Annotations:     []annotationWire{},
		Depends:         append([]string{}, task.Dependencies...),
		Recur:           task.Recurrence,
		Parent:          task.Parent,
		Urgency:         task.Urgency,
		Edit:            newEditFields(task, now.Location()),
	}
	if task.Estimate != nil {
		wire.EstimateMinutes = task.Estimate.Minutes
		wire.Estimate = task.Estimate.String()
	}
	if task.EstimateWarning != "" {
		var raw string
		if json.Unmarshal(task.RawFields["estimate"], &raw) == nil {
			wire.EstimateRaw = raw
		}
	}
	if task.Recurrence != "" {
		wire.RecurDisplay = domain.RecurrenceDisplay(task.Recurrence)
		wire.RecurrenceTarget = task.RecurrenceTargetUUID()
	}
	if task.IsPending() {
		group := domain.ClassifyToday(task, now)
		if group == "" && domain.IsPlannedFor(task, now) {
			group = domain.GroupPlannedToday
		}
		wire.TodayGroup = string(group)
	}
	for _, annotation := range task.Annotations {
		wire.Annotations = append(wire.Annotations, annotationWire{Description: annotation.Description, Entry: stamp(annotation.Entry)})
	}
	for key, raw := range task.RawFields {
		if knownFields[key] {
			continue
		}
		if wire.Extra == nil {
			wire.Extra = map[string]string{}
		}
		wire.Extra[key] = string(raw)
	}
	return wire
}

// newEditFields mirrors the TUI editor's initial values.
func newEditFields(task domain.Task, loc *time.Location) editFields {
	snapshot := domain.Snapshot(task)
	estimate := ""
	if snapshot.Estimate != nil {
		estimate = snapshot.Estimate.InputValue()
	}
	return editFields{
		Description: snapshot.Description,
		Project:     snapshot.Project,
		Priority:    snapshot.Priority,
		Due:         editableDate(task.Due, snapshot.Due, loc),
		Scheduled:   editableDate(task.Scheduled, snapshot.Scheduled, loc),
		Tags:        strings.Join(snapshot.Tags, " "),
		Estimate:    estimate,
		Recurrence:  snapshot.Recurrence,
	}
}

func editableDate(value *time.Time, fallback string, loc *time.Location) string {
	if value == nil {
		return fallback
	}
	return value.In(loc).Format("2006-01-02 15:04")
}

func newInterpretationWire(value quickadd.Interpretation, token int64) *interpretationWire {
	wire := &interpretationWire{
		Token:          token,
		Source:         value.Source,
		Valid:          value.Valid,
		RequiresReview: value.RequiresReview,
		Description:    value.Task.Description,
		Candidates:     []candidateWire{},
		Diagnostics:    []diagnosticWire{},
		ReviewValues:   quickadd.ReviewValues(value.Task),
	}
	for _, candidate := range value.Candidates {
		wire.Candidates = append(wire.Candidates, candidateWire{
			Field:      string(candidate.Field),
			Text:       candidate.Text,
			Value:      candidate.Value,
			Start:      candidate.Start,
			End:        candidate.End,
			Provenance: string(candidate.Provenance),
			Accepted:   candidate.Accepted,
			Conflict:   candidate.Conflict,
			Blocking:   candidate.Blocking,
			Diagnostic: candidate.Diagnostic,
		})
	}
	for _, diagnostic := range value.Diagnostics {
		wire.Diagnostics = append(wire.Diagnostics, diagnosticWire{Message: diagnostic.Message, Blocking: diagnostic.Blocking})
	}
	return wire
}

func stamp(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
