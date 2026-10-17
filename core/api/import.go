package api

// An import moves Taskwarrior's tasks to the server: every status, recurring
// templates and their instances, with their UUIDs, timestamps, annotations,
// dependencies, and, in Task.Extra, every field Momentum does not use
// (backend plan T16). It only creates tasks: one the user already has is
// left as it is, so importing again changes nothing.
//
// RoutePreviewImport answers what RouteApplyImport would do, and changes
// nothing. RouteApplyImport creates the tasks the user does not have, in one
// transaction, under an IdempotencyKeyHeader. RouteUndo refuses to revert an
// import, with CodeConflict.
const (
	RoutePreviewImport = "POST /v1/import/preview"
	RouteApplyImport   = "POST /v1/import/apply"
)

// ImportRequest is the body of both import routes. Each task is as
// Taskwarrior exported it; Urgency is ignored. A recurring template needs
// Due, and Recurrence must be one Momentum supports.
type ImportRequest struct {
	Tasks []Task `json:"tasks"`
}

// ImportResult answers both import routes with the UUIDs of the request's
// tasks, in the request's order, by what the import does with each.
type ImportResult struct {
	// Created are the tasks the import creates.
	Created []string `json:"created"`
	// Unchanged are tasks the user already has, equal in every field.
	Unchanged []string `json:"unchanged"`
	// Different are tasks the user already has with other values. The import
	// leaves them as they are.
	Different []ImportDifference `json:"different"`
}

// ImportDifference names the JSON fields in which a task the user already
// has differs from the one imported.
type ImportDifference struct {
	UUID   string   `json:"uuid"`
	Fields []string `json:"fields"`
}
