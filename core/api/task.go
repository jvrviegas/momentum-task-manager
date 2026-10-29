package api

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// Task statuses. A waiting task is pending with a wait date in the future.
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusDeleted   = "deleted"
	StatusRecurring = "recurring"
)

// Task is a task as the backend returns it. A recurring series is a template
// with StatusRecurring, which keeps the series' fields, and one pending
// instance at a time, which names the template in Parent and copies its
// Recurrence. Completing or deleting the instance creates the next one.
// Extra holds fields imported from Taskwarrior that Momentum does not use.
type Task struct {
	UUID            string                     `json:"uuid"`
	Description     string                     `json:"description"`
	Status          string                     `json:"status"`
	Project         string                     `json:"project,omitempty"`
	Priority        string                     `json:"priority,omitempty"`
	EstimateMinutes int                        `json:"estimateMinutes,omitempty"`
	Entry           time.Time                  `json:"entry"`
	Modified        time.Time                  `json:"modified"`
	Due             *time.Time                 `json:"due,omitempty"`
	Scheduled       *time.Time                 `json:"scheduled,omitempty"`
	Start           *time.Time                 `json:"start,omitempty"`
	Wait            *time.Time                 `json:"wait,omitempty"`
	Until           *time.Time                 `json:"until,omitempty"`
	End             *time.Time                 `json:"end,omitempty"`
	Tags            []string                   `json:"tags,omitempty"`
	Annotations     []Annotation               `json:"annotations,omitempty"`
	Depends         []string                   `json:"depends,omitempty"`
	Recurrence      string                     `json:"recurrence,omitempty"`
	Parent          string                     `json:"parent,omitempty"`
	Urgency         float64                    `json:"urgency"`
	Extra           map[string]json.RawMessage `json:"extra,omitempty"`
}

// Annotation is a dated note on a task.
type Annotation struct {
	Entry       time.Time `json:"entry"`
	Description string    `json:"description"`
}

// TaskList answers RouteListTasks.
type TaskList struct {
	Tasks []Task `json:"tasks"`
}

// CreateTaskRequest is the body of RouteCreateTask, the fields of a
// domain.NewTask. With Recurrence, which needs Due, it creates a recurring
// series due first at Due, and the answer is its first instance.
type CreateTaskRequest struct {
	Description     string     `json:"description"`
	Project         string     `json:"project,omitempty"`
	Priority        string     `json:"priority,omitempty"`
	Due             *time.Time `json:"due,omitempty"`
	Scheduled       *time.Time `json:"scheduled,omitempty"`
	Recurrence      string     `json:"recurrence,omitempty"`
	EstimateMinutes int        `json:"estimateMinutes,omitempty"`
	Tags            []string   `json:"tags,omitempty"`
}

// ModifyTaskRequest is the body of RouteModifyTask, the fields of a
// domain.TaskDiff. A field left out stays as it is, null clears it, and a
// value sets it. Recurrence on a task that does not repeat makes it the first
// instance of a new series; on an instance or a template, it changes the
// series' rule, and a template takes no other field. A recurrence cannot be
// cleared; RouteStopRecurrence ends a series instead. Annotations and Depends
// add and remove notes and dependencies, as Tags does tags.
type ModifyTaskRequest struct {
	Description     Change[string]    `json:"description,omitzero"`
	Project         Change[string]    `json:"project,omitzero"`
	Priority        Change[string]    `json:"priority,omitzero"`
	Due             Change[time.Time] `json:"due,omitzero"`
	Scheduled       Change[time.Time] `json:"scheduled,omitzero"`
	Recurrence      Change[string]    `json:"recurrence,omitzero"`
	EstimateMinutes Change[int]       `json:"estimateMinutes,omitzero"`
	Tags            *TagChange        `json:"tags,omitempty"`
	Annotations     *AnnotationChange `json:"annotations,omitempty"`
	Depends         *DependencyChange `json:"depends,omitempty"`
}

// Change is one field of a ModifyTaskRequest. Value is used only with
// domain.Set.
type Change[T any] struct {
	Kind  domain.ChangeKind
	Value T
}

// IsZero reports an unchanged field, which omitzero leaves out.
func (c Change[T]) IsZero() bool { return c.Kind == domain.Unchanged }

func (c Change[T]) MarshalJSON() ([]byte, error) {
	if c.Kind == domain.Clear {
		return []byte("null"), nil
	}
	return json.Marshal(c.Value)
}

func (c *Change[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*c = Change[T]{Kind: domain.Clear}
		return nil
	}
	*c = Change[T]{Kind: domain.Set}
	return json.Unmarshal(data, &c.Value)
}

// TagChange adds and removes tags, leaving the task's other tags alone.
type TagChange struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// AnnotationChange adds notes, by their words, and removes notes, by their
// time and words. The server gives an added note its time.
type AnnotationChange struct {
	Add    []string     `json:"add,omitempty"`
	Remove []Annotation `json:"remove,omitempty"`
}

// DependencyChange adds and removes the tasks a task depends on, by UUID.
type DependencyChange struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}
