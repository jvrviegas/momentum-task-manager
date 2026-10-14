package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// ProjectDraft is the editable form of a catalog entry. ParentValue is the
// complete configured parent path, while ValueSegment is this entry's local
// path segment. The persisted Project contains only the composed full value.
type ProjectDraft struct {
	Name         string
	ParentValue  string
	ValueSegment string
}

var projectValueSegmentPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ComposeProjectValue combines a complete parent value with one local value
// segment. Parent entries do not need to be present in the catalog.
func ComposeProjectValue(parentValue, valueSegment string) (string, error) {
	if parentValue != "" && !projectValuePattern.MatchString(parentValue) {
		return "", fmt.Errorf("project parent value %q is invalid", parentValue)
	}
	if !projectValueSegmentPattern.MatchString(valueSegment) {
		return "", fmt.Errorf("project value segment %q is invalid", valueSegment)
	}
	if parentValue == "" {
		return valueSegment, nil
	}
	return parentValue + "." + valueSegment, nil
}

// Project resolves a draft into the one value stored by the catalog.
func (d ProjectDraft) Project() (Project, error) {
	value, err := ComposeProjectValue(d.ParentValue, d.ValueSegment)
	if err != nil {
		return Project{}, err
	}
	project := Project{Name: d.Name, Value: value}
	if err := (ProjectCatalog{project}).Validate(); err != nil {
		return Project{}, err
	}
	return project, nil
}

// DraftForProject splits a persisted value at its final segment for editing.
func DraftForProject(project Project) ProjectDraft {
	parent, segment := "", project.Value
	if dot := strings.LastIndexByte(project.Value, '.'); dot >= 0 {
		parent, segment = project.Value[:dot], project.Value[dot+1:]
	}
	return ProjectDraft{Name: project.Name, ParentValue: parent, ValueSegment: segment}
}

// ProjectEditKind identifies the catalog operation represented by a plan.
type ProjectEditKind string

const (
	ProjectEditNoOp      ProjectEditKind = "no-op"
	ProjectEditAdd       ProjectEditKind = "add"
	ProjectEditLabelOnly ProjectEditKind = "label-only"
	ProjectEditValue     ProjectEditKind = "value"
	ProjectEditRemove    ProjectEditKind = "remove"
)

// ProjectValueMapping is an explicit old-to-new catalog or task value map.
type ProjectValueMapping struct {
	OldValue string
	NewValue string
}

// ProjectCatalogPlan is an immutable snapshot of one proposed catalog edit.
// Before and After are copied so callers can safely retain a preview while the
// live catalog remains unchanged.
type ProjectCatalogPlan struct {
	Kind               ProjectEditKind
	Before             ProjectCatalog
	After              ProjectCatalog
	Source             Project
	Destination        Project
	SourceValue        string
	DestinationValue   string
	IncludeSubprojects bool
	RemoveChildren     bool
	Mappings           []ProjectValueMapping
}

// Changed reports whether applying the plan would alter the catalog.
func (p ProjectCatalogPlan) Changed() bool { return p.Kind != ProjectEditNoOp }

// ValueChanged reports whether the plan changes a stored Taskwarrior value.
func (p ProjectCatalogPlan) ValueChanged() bool { return len(p.Mappings) > 0 }

// PlanAddProject validates and plans appending one catalog entry.
func PlanAddProject(catalog ProjectCatalog, project Project) (ProjectCatalogPlan, error) {
	if err := catalog.Validate(); err != nil {
		return ProjectCatalogPlan{}, err
	}
	if err := (ProjectCatalog{project}).Validate(); err != nil {
		return ProjectCatalogPlan{}, err
	}
	before := cloneProjectCatalog(catalog)
	after := append(cloneProjectCatalog(catalog), project)
	if err := after.Validate(); err != nil {
		return ProjectCatalogPlan{}, err
	}
	return ProjectCatalogPlan{
		Kind:             ProjectEditAdd,
		Before:           before,
		After:            after,
		Destination:      project,
		DestinationValue: project.Value,
	}, nil
}

// PlanUpdateProject validates and plans replacing the exact source entry. If
// IncludeSubprojects is true, configured descendants are remapped by suffix.
func PlanUpdateProject(catalog ProjectCatalog, sourceValue string, destination Project, includeSubprojects bool) (ProjectCatalogPlan, error) {
	if err := catalog.Validate(); err != nil {
		return ProjectCatalogPlan{}, err
	}
	if err := (ProjectCatalog{destination}).Validate(); err != nil {
		return ProjectCatalogPlan{}, err
	}
	sourceIndex, source, ok := catalog.projectByValue(sourceValue)
	if !ok {
		return ProjectCatalogPlan{}, fmt.Errorf("project value %q is not configured", sourceValue)
	}
	plan := ProjectCatalogPlan{
		Before:             cloneProjectCatalog(catalog),
		Source:             source,
		Destination:        destination,
		SourceValue:        source.Value,
		DestinationValue:   destination.Value,
		IncludeSubprojects: includeSubprojects,
	}

	if source.Value == destination.Value {
		plan.After = cloneProjectCatalog(catalog)
		plan.After[sourceIndex] = destination
		if source.Name == destination.Name {
			plan.Kind = ProjectEditNoOp
		} else {
			plan.Kind = ProjectEditLabelOnly
		}
		return plan, nil
	}
	if isProjectDescendant(destination.Value, source.Value) {
		return ProjectCatalogPlan{}, fmt.Errorf("cannot move project %q under its own descendant %q", source.Value, destination.Value)
	}

	after := cloneProjectCatalog(catalog)
	after[sourceIndex] = destination
	mappings := []ProjectValueMapping{{OldValue: source.Value, NewValue: destination.Value}}
	if includeSubprojects {
		for i := range after {
			if i == sourceIndex || !isProjectDescendant(after[i].Value, source.Value) {
				continue
			}
			suffix := strings.TrimPrefix(after[i].Value, source.Value)
			mappings = append(mappings, ProjectValueMapping{
				OldValue: after[i].Value,
				NewValue: destination.Value + suffix,
			})
			after[i].Value = destination.Value + suffix
		}
	}
	if err := after.Validate(); err != nil {
		return ProjectCatalogPlan{}, fmt.Errorf("plan project update: %w", err)
	}
	plan.Kind = ProjectEditValue
	plan.After = after
	plan.Mappings = mappings
	return plan, nil
}

// PlanRemoveProject validates and plans removing the exact source entry. When
// RemoveChildren is true, every configured dotted descendant is removed too.
func PlanRemoveProject(catalog ProjectCatalog, sourceValue string, removeChildren bool) (ProjectCatalogPlan, error) {
	if err := catalog.Validate(); err != nil {
		return ProjectCatalogPlan{}, err
	}
	_, source, ok := catalog.projectByValue(sourceValue)
	if !ok {
		return ProjectCatalogPlan{}, fmt.Errorf("project value %q is not configured", sourceValue)
	}
	before := cloneProjectCatalog(catalog)
	after := make(ProjectCatalog, 0, len(catalog))
	for _, project := range catalog {
		if project.Value == sourceValue || (removeChildren && isProjectDescendant(project.Value, sourceValue)) {
			continue
		}
		after = append(after, project)
	}
	return ProjectCatalogPlan{
		Kind:           ProjectEditRemove,
		Before:         before,
		After:          after,
		Source:         source,
		SourceValue:    source.Value,
		RemoveChildren: removeChildren,
	}, nil
}

// PlanAddDraft resolves and plans a new editable entry.
func PlanAddDraft(catalog ProjectCatalog, draft ProjectDraft) (ProjectCatalogPlan, error) {
	project, err := draft.Project()
	if err != nil {
		return ProjectCatalogPlan{}, err
	}
	return PlanAddProject(catalog, project)
}

// PlanUpdateDraft resolves and plans an editable entry replacement.
func PlanUpdateDraft(catalog ProjectCatalog, sourceValue string, draft ProjectDraft, includeSubprojects bool) (ProjectCatalogPlan, error) {
	project, err := draft.Project()
	if err != nil {
		return ProjectCatalogPlan{}, err
	}
	return PlanUpdateProject(catalog, sourceValue, project, includeSubprojects)
}

func (c ProjectCatalog) projectByValue(value string) (int, Project, bool) {
	for i, project := range c {
		if project.Value == value {
			return i, project, true
		}
	}
	return 0, Project{}, false
}

func cloneProjectCatalog(catalog ProjectCatalog) ProjectCatalog {
	if catalog == nil {
		return nil
	}
	return append(ProjectCatalog(nil), catalog...)
}

func isProjectDescendant(value, parent string) bool {
	return parent != "" && strings.HasPrefix(value, parent+".")
}
