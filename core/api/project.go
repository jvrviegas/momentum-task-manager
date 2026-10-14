package api

// ProjectList answers RouteListProjects with the projects of pending tasks.
type ProjectList struct {
	Projects []string `json:"projects"`
}

// TagList answers RouteListTags with the tags of pending tasks.
type TagList struct {
	Tags []string `json:"tags"`
}

// ProjectMigration is the body of RoutePreviewProjectMigration: move pending
// tasks from Source to Destination and, with IncludeSubprojects, from Source's
// dotted subprojects to the same subprojects of Destination (ADR 0001).
type ProjectMigration struct {
	Source             string `json:"source"`
	Destination        string `json:"destination"`
	IncludeSubprojects bool   `json:"includeSubprojects"`
}

// ProjectMigrationPreview answers RoutePreviewProjectMigration with the tasks
// the migration would change.
type ProjectMigrationPreview struct {
	Tasks []ProjectTaskMapping `json:"tasks"`
}

// ProjectTaskMapping is one task's project before and after a migration.
type ProjectTaskMapping struct {
	UUID     string `json:"uuid"`
	OldValue string `json:"oldValue"`
	NewValue string `json:"newValue"`
}

// ApplyProjectMigrationRequest is the body of RouteApplyProjectMigration: the
// migration and the preview's tasks. The server changes all of them in one
// transaction, or none with CodeConflict when the migration would no longer
// change exactly those tasks.
type ApplyProjectMigrationRequest struct {
	ProjectMigration
	Tasks []ProjectTaskMapping `json:"tasks"`
}

// ProjectMigrationResult answers RouteApplyProjectMigration.
type ProjectMigrationResult struct {
	Changed int `json:"changed"`
}
