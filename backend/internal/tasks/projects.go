package tasks

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// A project migration moves tasks from one project to another, as ADR 0001
// does in Taskwarrior mode, but over all of the user's tasks, because
// backend mode has no contexts (backend plan D6). It reaches the pending
// tasks that are not waiting and do not repeat, whose project is the source
// or, with subprojects included, one of its dotted subprojects. The apps keep
// the project catalog and check its rules before they migrate.

// migrateAction names the task_events rows of a project migration.
const migrateAction = "migrate-project"

var errStaleMigration = conflict("the tasks to migrate have changed; preview the migration again")

// PreviewProjectMigration answers api.RoutePreviewProjectMigration with the
// tasks migration would change, oldest first.
func (s *Service) PreviewProjectMigration(ctx context.Context, migration api.ProjectMigration) (api.ProjectMigrationPreview, error) {
	if err := validateMigration(migration); err != nil {
		return api.ProjectMigrationPreview{}, err
	}
	rows, err := migrating(s.client, migration).Order(task.ByEntry(), task.ByID()).All(ctx)
	if err != nil {
		return api.ProjectMigrationPreview{}, err
	}
	return api.ProjectMigrationPreview{Tasks: mappings(rows, migration, s.clock())}, nil
}

// ApplyProjectMigration moves the tasks of request to their new projects in
// one change, under the idempotency key, and answers with how many it
// changed. If the migration would no longer change exactly the tasks the
// request lists, as their preview showed them, it changes none and answers
// a conflict.
func (s *Service) ApplyProjectMigration(ctx context.Context, key string, request api.ApplyProjectMigrationRequest) (api.ProjectMigrationResult, error) {
	if err := validateMigration(request.ProjectMigration); err != nil {
		return api.ProjectMigrationResult{}, err
	}
	return once(ctx, s, key, migrateAction, uuid.Nil, request, func(c *change) (api.ProjectMigrationResult, error) {
		// Locking in id order, as undo does, keeps the two from deadlocking.
		rows, err := migrating(c.tx, request.ProjectMigration).Order(task.ByID()).ForUpdate().All(ctx)
		if err != nil {
			return api.ProjectMigrationResult{}, err
		}
		current := mappings(rows, request.ProjectMigration, c.now)
		if !sameMappings(current, request.Tasks) {
			return api.ProjectMigrationResult{}, errStaleMigration
		}
		projects := make(map[string]string, len(current))
		for _, mapping := range current {
			projects[mapping.UUID] = mapping.NewValue
		}
		for _, row := range rows {
			project, ok := projects[row.ID.String()]
			if !ok {
				continue
			}
			_, err := c.update(ctx, migrateAction, row, func(t *api.Task, _ time.Time) error {
				t.Project = project
				return nil
			})
			if err != nil {
				return api.ProjectMigrationResult{}, err
			}
		}
		return api.ProjectMigrationResult{Changed: len(current)}, nil
	})
}

func validateMigration(migration api.ProjectMigration) error {
	switch {
	case migration.Source == "":
		return invalid("source", "source cannot be empty")
	case migration.Destination == "":
		return invalid("destination", "destination cannot be empty")
	case migration.Destination == migration.Source:
		return invalid("destination", "destination must differ from source")
	}
	return nil
}

// migrating queries the pending tasks that do not repeat and whose project
// migration moves. Waiting tasks are left to mappings, which knows the time.
func migrating(client *ent.Client, migration api.ProjectMigration) *ent.TaskQuery {
	project := task.Project(migration.Source)
	if migration.IncludeSubprojects {
		project = task.Or(project, task.ProjectHasPrefix(migration.Source+"."))
	}
	return client.Task.Query().Where(task.StatusEQ(task.StatusPending), task.Recurrence(""), project)
}

// mappings returns the project each of rows that is not waiting at now moves
// to, with domain.MapPendingProjectTasks, as Taskwarrior mode maps them.
func mappings(rows []*ent.Task, migration api.ProjectMigration, now time.Time) []api.ProjectTaskMapping {
	tasks := make([]domain.Task, 0, len(rows))
	for _, row := range rows {
		if !waiting(row, now) {
			tasks = append(tasks, domain.Task{UUID: row.ID.String(), Status: string(row.Status), Project: row.Project, Recurrence: row.Recurrence})
		}
	}
	mapped := domain.MapPendingProjectTasks(tasks, migration.Source, migration.Destination, migration.IncludeSubprojects)
	result := make([]api.ProjectTaskMapping, len(mapped))
	for i, mapping := range mapped {
		result[i] = api.ProjectTaskMapping(mapping)
	}
	return result
}

// sameMappings reports whether sent lists exactly the mappings of current,
// in any order and each once.
func sameMappings(current, sent []api.ProjectTaskMapping) bool {
	if len(sent) != len(current) {
		return false
	}
	unsent := make(map[api.ProjectTaskMapping]bool, len(current))
	for _, mapping := range current {
		unsent[mapping] = true
	}
	for _, mapping := range sent {
		if !unsent[mapping] {
			return false
		}
		delete(unsent, mapping)
	}
	return true
}
