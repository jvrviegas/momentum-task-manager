package api

// Routes are net/http ServeMux patterns. A change that breaks existing clients
// gets a new version prefix instead of changing these. Each task mutation,
// and RouteUndo, answers with the Task as it is afterwards.
//
// RouteUndo reverts the latest change the signed-in device made, if its tasks
// have not changed since; otherwise, or after an undo, it answers
// CodeConflict. A change can reach several tasks, such as completing an
// instance and creating the next one, and undo reverts all of them. Undoing
// a create removes the task, so the answer is then its last state with
// StatusDeleted.
//
// RouteStopRecurrence ends the series of the task in its path, a template or
// one of its instances: the template's Until becomes the start of today in
// the user's time zone, and the pending instance stays as the last one. It
// answers with the template, or CodeConflict for a task that does not repeat
// or a series already stopped.
//
// RouteSetTimeZone sets the user's time zone, which anchors their recurring
// series, and answers with it. It needs no IdempotencyKeyHeader, because
// sending the same zone again changes nothing.
const (
	RouteVersion                 = "GET /v1/version"
	RouteListTasks               = "GET /v1/tasks"
	RouteCreateTask              = "POST /v1/tasks"
	RouteModifyTask              = "PATCH /v1/tasks/{uuid}"
	RouteDeleteTask              = "DELETE /v1/tasks/{uuid}"
	RouteCompleteTask            = "POST /v1/tasks/{uuid}/complete"
	RouteStartTask               = "POST /v1/tasks/{uuid}/start"
	RouteStopTask                = "POST /v1/tasks/{uuid}/stop"
	RouteStopRecurrence          = "POST /v1/tasks/{uuid}/stop-recurrence"
	RouteUndo                    = "POST /v1/undo"
	RouteListProjects            = "GET /v1/projects"
	RouteListTags                = "GET /v1/tags"
	RoutePreviewProjectMigration = "POST /v1/project-migrations/preview"
	RouteApplyProjectMigration   = "POST /v1/project-migrations/apply"
	RouteSetTimeZone             = "PUT /v1/time-zone"
)

// PathUUID is the wildcard in the task routes.
const PathUUID = "uuid"

// Query parameters of RouteListTasks. QueryStatus is StatusPending,
// StatusRecurring, or StatusCompleted. Pending excludes tasks whose wait date
// is in the future, as Taskwarrior's status:pending does. Completed needs
// QueryEndedAfter, an RFC 3339 instant.
const (
	QueryStatus     = "status"
	QueryEndedAfter = "ended_after"
)

// IdempotencyKeyHeader carries a key the client generates once per mutation
// and repeats on each retry of it. The server applies a key once and answers
// a retry with the first response.
const IdempotencyKeyHeader = "Idempotency-Key"

// Version answers RouteVersion.
type Version struct {
	Server string `json:"server"`
}

// TimeZone is the request and answer of RouteSetTimeZone: an IANA name such
// as "Europe/Lisbon".
type TimeZone struct {
	TimeZone string `json:"timeZone"`
}
