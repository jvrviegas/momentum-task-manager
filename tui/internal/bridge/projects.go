package bridge

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// CodeStalePreview: the tasks a rename would move changed since the preview,
// so nothing was changed.
const CodeStalePreview = "stalePreview"

// projectEntryWire is one row of the Projects tab. Tag says whether the
// value is in the catalog or only found on tasks; Tasks counts the pending
// tasks that have exactly this value.
type projectEntryWire struct {
	Value string `json:"value"`
	Name  string `json:"name"`
	Tag   string `json:"tag"`
	Tasks int    `json:"tasks"`
}

type projectsResult struct {
	// Revision names the version of config.toml the catalog was read from.
	Revision string             `json:"revision"`
	Entries  []projectEntryWire `json:"entries"`
	// TasksError says why the tasks could not be read, which leaves out their
	// counts and the values found only on tasks.
	TasksError string `json:"tasksError,omitempty"`
}

// projectEditParams is one change to the catalog: add an entry, edit the one
// at Source, or remove it. Name, Parent and Segment are the draft of an add or
// an edit, and the entry's value is Parent and Segment joined by a dot.
type projectEditParams struct {
	Revision           string `json:"revision"`
	Action             string `json:"action"`
	Source             string `json:"source"`
	Name               string `json:"name"`
	Parent             string `json:"parent"`
	Segment            string `json:"segment"`
	IncludeSubprojects bool   `json:"includeSubprojects"`
	RemoveChildren     bool   `json:"removeChildren"`
	// Migrate moves the pending tasks in Tasks, the ones the preview showed,
	// to the new value. Without it only the catalog changes.
	Migrate bool              `json:"migrate"`
	Tasks   []projectTaskWire `json:"tasks"`
}

// projectTaskWire is one task's project before and after a rename. It holds
// no title.
type projectTaskWire struct {
	UUID     string `json:"uuid"`
	OldValue string `json:"oldValue"`
	NewValue string `json:"newValue"`
}

// projectPreviewWire is what a rename would do. The two task lists are for
// Include subprojects off and on.
type projectPreviewWire struct {
	// Value is the entry's new full value, and ValueChanged whether it
	// differs from the old one: only then are tasks offered.
	Value        string `json:"value"`
	ValueChanged bool   `json:"valueChanged"`
	// CatalogDescendants are the configured entries under the old value that
	// Include subprojects renames with it.
	CatalogDescendants int               `json:"catalogDescendants"`
	ExactTasks         []projectTaskWire `json:"exactTasks"`
	SubprojectTasks    []projectTaskWire `json:"subprojectTasks"`
	// DestinationTaskOnly: tasks already use the new value, which the catalog
	// lacks, so the rename merges the moved tasks into them.
	DestinationTaskOnly bool `json:"destinationTaskOnly"`
}

type saveProjectResult struct {
	// Revision names config.toml as it was saved.
	Revision string `json:"revision"`
	// Changed is how many tasks moved.
	Changed int `json:"changed"`
	// TaskError says why the server moved no task after the catalog was saved.
	TaskError string `json:"taskError,omitempty"`
	// Stale: the tasks changed since the preview, so a new preview is needed.
	Stale bool `json:"stale,omitempty"`
}

// projectList lists the catalog and the values found only on tasks in one list,
// parents before their children.
func (s *Server) projectList(ctx context.Context) (*projectsResult, error) {
	result := &projectsResult{Entries: []projectEntryWire{}}
	if snapshot, err := config.NewProjectCatalogStore(s.configPath).Read(ctx); err == nil {
		result.Revision = config.RevisionID(snapshot.Revision)
	}
	counts := map[string]int{}
	pending, err := s.client.ExportPending(ctx)
	if err != nil {
		result.TasksError = err.Error()
	}
	for _, task := range pending {
		if task.Project != "" {
			counts[task.Project]++
		}
	}
	inCatalog := map[string]bool{}
	for _, project := range s.config.Projects {
		inCatalog[project.Value] = true
		result.Entries = append(result.Entries, projectEntryWire{Value: project.Value, Name: project.Name, Tag: "catalog", Tasks: counts[project.Value]})
	}
	for value, count := range counts {
		if !inCatalog[value] {
			result.Entries = append(result.Entries, projectEntryWire{Value: value, Name: value[strings.LastIndexByte(value, '.')+1:], Tag: "tasks", Tasks: count})
		}
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		return pathLess(result.Entries[i].Value, result.Entries[j].Value)
	})
	return result, nil
}

// pathLess orders values segment by segment, so work.client follows work and
// comes before work-extra.
func pathLess(a, b string) bool {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] != right[i] {
			return left[i] < right[i]
		}
	}
	return len(left) < len(right)
}

// previewMigration checks an edit that changes an entry's value and says
// which pending tasks the rename would move, with and without the entry's
// subprojects. It writes nothing.
func (s *Server) previewMigration(ctx context.Context, p projectEditParams) (*projectPreviewWire, error) {
	snapshot, err := config.NewProjectCatalogStore(s.configPath).Read(ctx)
	if err != nil {
		return nil, &Error{Message: "Unable to read config.toml. " + err.Error()}
	}
	exact, refusal := planProject(snapshot.Projects, p, false)
	if refusal != nil {
		return nil, refusal
	}
	preview := &projectPreviewWire{Value: exact.DestinationValue, ValueChanged: exact.ValueChanged(), ExactTasks: []projectTaskWire{}, SubprojectTasks: []projectTaskWire{}}
	if !preview.ValueChanged {
		return preview, nil
	}
	subprojects, refusal := planProject(snapshot.Projects, p, true)
	if refusal != nil {
		return nil, refusal
	}
	pending, err := s.client.ExportPending(ctx)
	if err != nil {
		return nil, err
	}
	preview.CatalogDescendants = len(subprojects.Mappings) - 1
	preview.ExactTasks = taskWires(exact.PendingTaskMappings(pending))
	preview.SubprojectTasks = taskWires(subprojects.PendingTaskMappings(pending))
	inCatalog := false
	for _, project := range snapshot.Projects {
		inCatalog = inCatalog || project.Value == exact.DestinationValue
	}
	if !inCatalog {
		for _, task := range pending {
			if task.Project == exact.DestinationValue || strings.HasPrefix(task.Project, exact.DestinationValue+".") {
				preview.DestinationTaskOnly = true
				break
			}
		}
	}
	return preview, nil
}

// saveProject changes the catalog and, when asked, moves the confirmed
// tasks. Nothing is written if the draft is refused, the file changed since
// the tab read it, or the tasks are no longer the ones the preview showed.
// The catalog is saved before the tasks move; if the server then refuses,
// the catalog stays saved and the result says why.
func (s *Server) saveProject(ctx context.Context, p projectEditParams) (*saveProjectResult, error) {
	if s.configError != "" {
		return nil, &Error{Message: "Fix config.toml first. The app is still using the last settings that worked."}
	}
	store := config.NewProjectCatalogStore(s.configPath)
	snapshot, err := store.Read(ctx)
	if err != nil {
		return nil, &Error{Message: "Unable to read config.toml. " + err.Error()}
	}
	if config.RevisionID(snapshot.Revision) != p.Revision {
		return nil, configConflict()
	}
	plan, refusal := planProject(snapshot.Projects, p, p.IncludeSubprojects)
	if refusal != nil {
		return nil, refusal
	}
	tasks := make([]domain.ProjectTaskMapping, len(p.Tasks))
	for i, task := range p.Tasks {
		tasks[i] = domain.ProjectTaskMapping(task)
	}
	migrate := p.Migrate && plan.ValueChanged()
	if migrate {
		pending, err := s.client.ExportPending(ctx)
		if err != nil {
			return nil, err
		}
		if !sameMappings(plan.PendingTaskMappings(pending), tasks) {
			return nil, &Error{Message: "Your tasks changed since the preview. Preview the rename again.", Code: CodeStalePreview}
		}
	}
	result := &saveProjectResult{Revision: p.Revision}
	if plan.Changed() {
		saved, err := store.Save(ctx, snapshot, plan.After)
		switch {
		case errors.Is(err, config.ErrProjectConfigConflict):
			return nil, configConflict()
		case err != nil:
			return nil, &Error{Message: "Unable to save config.toml. " + err.Error()}
		}
		result.Revision = config.RevisionID(saved.Revision)
		s.reloadConfig()
	}
	if !migrate || len(tasks) == 0 {
		return result, nil
	}
	err = s.client.ApplyProjectMigration(ctx, plan, tasks)
	var failure *api.Error
	switch {
	case errors.As(err, &failure) && failure.Code == api.CodeConflict:
		result.Stale = true
		result.TaskError = "Your tasks changed since the preview, so none moved. The catalog is saved; preview the rename again to move them."
	case err != nil:
		result.TaskError = err.Error()
	default:
		result.Changed = len(tasks)
	}
	return result, nil
}

// planProject checks a draft against the catalog and plans the change, with a
// message for the field that holds a refused value.
func planProject(catalog domain.ProjectCatalog, p projectEditParams, includeSubprojects bool) (domain.ProjectCatalogPlan, *Error) {
	if p.Action == "remove" {
		plan, err := domain.PlanRemoveProject(catalog, p.Source, p.RemoveChildren)
		if err != nil {
			return plan, &Error{Message: "That project isn’t in the catalog any more."}
		}
		return plan, nil
	}
	if p.Action != "add" && p.Action != "edit" {
		return domain.ProjectCatalogPlan{}, &Error{Message: "Unknown project change " + p.Action + "."}
	}
	name := strings.TrimSpace(p.Name)
	if name == "" || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return domain.ProjectCatalogPlan{}, fieldError("name", "Enter a name.")
	}
	if _, err := domain.ComposeProjectValue("", p.Segment); err != nil {
		return domain.ProjectCatalogPlan{}, fieldError("segment", "Use lowercase letters, numbers and hyphens, such as client-name.")
	}
	value, err := domain.ComposeProjectValue(p.Parent, p.Segment)
	if err != nil {
		return domain.ProjectCatalogPlan{}, fieldError("parent", "Choose a parent from the list.")
	}
	for _, project := range catalog {
		if project.Value == value && project.Value != p.Source {
			return domain.ProjectCatalogPlan{}, fieldError("segment", "“"+value+"” is already in the catalog.")
		}
	}
	draft := domain.ProjectDraft{Name: name, ParentValue: p.Parent, ValueSegment: p.Segment}
	var plan domain.ProjectCatalogPlan
	if p.Action == "add" {
		plan, err = domain.PlanAddDraft(catalog, draft)
	} else {
		plan, err = domain.PlanUpdateDraft(catalog, p.Source, draft, includeSubprojects)
	}
	switch {
	case err == nil:
		return plan, nil
	case strings.Contains(err.Error(), "its own descendant"):
		return plan, fieldError("parent", "A project can’t move under itself.")
	case strings.Contains(err.Error(), "is not configured"):
		return plan, &Error{Message: "That project isn’t in the catalog any more."}
	case strings.Contains(err.Error(), "duplicates"):
		return plan, fieldError("segment", "That rename would give two entries the same value. Rename or remove the other one first.")
	default:
		return plan, &Error{Message: err.Error()}
	}
}

func taskWires(mappings []domain.ProjectTaskMapping) []projectTaskWire {
	wires := make([]projectTaskWire, len(mappings))
	for i, mapping := range mappings {
		wires[i] = projectTaskWire(mapping)
	}
	return wires
}

// sameMappings reports whether both lists name the same tasks with the same
// values, whatever their order.
func sameMappings(current, confirmed []domain.ProjectTaskMapping) bool {
	if len(current) != len(confirmed) {
		return false
	}
	byUUID := make(map[string]domain.ProjectTaskMapping, len(current))
	for _, mapping := range current {
		byUUID[mapping.UUID] = mapping
	}
	for _, mapping := range confirmed {
		if byUUID[mapping.UUID] != mapping {
			return false
		}
	}
	return true
}
