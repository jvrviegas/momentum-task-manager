package domain

import "strings"

// ProjectTaskMapping identifies one task by UUID and records its observed
// source value and planned destination value.
type ProjectTaskMapping struct {
	UUID     string
	OldValue string
	NewValue string
}

// TaskProjectMapping is a descriptive alias for callers that lead with the
// task in the name.
type TaskProjectMapping = ProjectTaskMapping

// PendingTaskMappings plans exact or dotted-descendant changes for eligible
// pending task snapshots. It never mutates the supplied tasks.
func (p ProjectCatalogPlan) PendingTaskMappings(tasks []Task) []ProjectTaskMapping {
	if !p.ValueChanged() {
		return nil
	}
	return MapPendingProjectTasks(tasks, p.SourceValue, p.DestinationValue, p.IncludeSubprojects)
}

// MapPendingProjectTasks maps only pending, non-recurring tasks whose project
// exactly matches sourceValue. With IncludeSubprojects, a dot-delimited
// descendant is mapped by preserving the suffix; prefix lookalikes do not
// match.
func MapPendingProjectTasks(tasks []Task, sourceValue, destinationValue string, includeSubprojects bool) []ProjectTaskMapping {
	if sourceValue == "" || destinationValue == "" || sourceValue == destinationValue {
		return nil
	}
	mappings := make([]ProjectTaskMapping, 0)
	for _, task := range tasks {
		if task.UUID == "" || !task.IsPending() || task.Recurrence != "" {
			continue
		}
		newValue, ok := migratedProjectValue(task.Project, sourceValue, destinationValue, includeSubprojects)
		if !ok {
			continue
		}
		mappings = append(mappings, ProjectTaskMapping{
			UUID:     task.UUID,
			OldValue: task.Project,
			NewValue: newValue,
		})
	}
	return mappings
}

func migratedProjectValue(value, sourceValue, destinationValue string, includeSubprojects bool) (string, bool) {
	if value == sourceValue {
		return destinationValue, true
	}
	if !includeSubprojects || !strings.HasPrefix(value, sourceValue+".") {
		return "", false
	}
	return destinationValue + strings.TrimPrefix(value, sourceValue), true
}
