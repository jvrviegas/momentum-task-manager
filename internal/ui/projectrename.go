package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
)

// ProjectRenameMode selects whether a value edit saves the catalog only or
// also applies the already-previewed pending-task mappings.
type ProjectRenameMode string

const (
	ProjectRenameCatalogOnly       ProjectRenameMode = "catalog-only"
	ProjectRenameCatalogAndPending ProjectRenameMode = "catalog-and-pending"
)

// ProjectRenamePreview is the coordinator's immutable preview snapshot. The
// exact and subtree plans are supplied separately so changing scope cannot
// silently reuse an earlier mapping.
type ProjectRenamePreview struct {
	ExactPlan       domain.ProjectCatalogPlan
	SubprojectsPlan domain.ProjectCatalogPlan
	ExactTasks      []domain.ProjectTaskMapping
	SubprojectTasks []domain.ProjectTaskMapping

	ContextName         string
	ReadFilter          string
	HasActiveContext    bool
	DestinationTaskOnly bool
	CollisionMessage    string
}

// ProjectRenameOptionMsg requests a fresh preview after an option changes.
type ProjectRenameOptionMsg struct {
	Mode               ProjectRenameMode
	IncludeSubprojects bool
}

// ProjectRenameConfirmMsg is emitted only after explicit confirmation of a
// valid preview.
type ProjectRenameConfirmMsg struct {
	Preview            ProjectRenamePreview
	Plan               domain.ProjectCatalogPlan
	Mode               ProjectRenameMode
	IncludeSubprojects bool
	TaskMappings       []domain.ProjectTaskMapping
}

// ProjectRenameCancelMsg reports an explicit cancellation.
type ProjectRenameCancelMsg struct{}

// ProjectRenameResultMsg reports execution feedback. Stale results invalidate
// the preview but leave the component open so the user can obtain a fresh one.
type ProjectRenameResultMsg struct {
	Stale bool
	Err   error
}

// ProjectRenameModel renders and guards the explicit rename preview flow.
type ProjectRenameModel struct {
	Open               bool
	Preview            ProjectRenamePreview
	Mode               ProjectRenameMode
	IncludeSubprojects bool
	PreviewValid       bool
	ConfirmOpen        bool
	MergeWarningOpen   bool
	Err                error
	Width              int
	Height             int
	Styles             Styles
}

// NewProjectRename creates a closed preview component.
func NewProjectRename(styles Styles) ProjectRenameModel {
	return ProjectRenameModel{Styles: styles, Mode: ProjectRenameCatalogOnly}
}

func (p *ProjectRenameModel) SetSize(width, height int) {
	p.Width, p.Height = width, height
}

// OpenPreview resets migration consent for each value edit.
func (p *ProjectRenameModel) OpenPreview(preview ProjectRenamePreview) {
	p.Open = true
	p.Preview = cloneProjectRenamePreview(preview)
	p.Mode = ProjectRenameCatalogOnly
	p.IncludeSubprojects = false
	p.PreviewValid = true
	p.ConfirmOpen = false
	p.MergeWarningOpen = false
	p.Err = nil
}

// UpdatePreview installs a fresh preview while preserving the current mode and
// descendant choice that caused the recomputation.
func (p *ProjectRenameModel) UpdatePreview(preview ProjectRenamePreview) {
	p.Preview = cloneProjectRenamePreview(preview)
	p.PreviewValid = true
	p.ConfirmOpen = false
	p.MergeWarningOpen = false
	p.Err = nil
}

// Close cancels the preview without emitting a task operation.
func (p *ProjectRenameModel) Close() {
	p.Open = false
	p.ConfirmOpen = false
	p.MergeWarningOpen = false
	p.Err = nil
}

// ApplyMessage keeps stale or failed execution actionable and closes only after
// an explicitly successful result.
func (p *ProjectRenameModel) ApplyMessage(msg tea.Msg) (tea.Msg, bool) {
	result, ok := msg.(ProjectRenameResultMsg)
	if !ok {
		return nil, false
	}
	p.ConfirmOpen = false
	p.MergeWarningOpen = false
	if result.Stale {
		p.PreviewValid = false
	}
	if result.Err != nil {
		p.Err = result.Err
		return nil, true
	}
	p.Close()
	return result, true
}

// CanMigrateTasks reports whether the current valid preview can offer task
// migration.
func (p ProjectRenameModel) CanMigrateTasks() bool {
	return p.Open && p.PreviewValid && p.Mode == ProjectRenameCatalogAndPending && p.activePlan().ValueChanged()
}

// ActivePlan returns the plan selected by the current descendant option.
func (p ProjectRenameModel) ActivePlan() domain.ProjectCatalogPlan {
	return p.activePlan()
}

// ActiveTaskMappings returns a copied mapping list for the selected scope.
func (p ProjectRenameModel) ActiveTaskMappings() []domain.ProjectTaskMapping {
	return append([]domain.ProjectTaskMapping(nil), p.activeTasks()...)
}

func (p *ProjectRenameModel) Update(msg tea.Msg) (*ProjectRenameModel, tea.Cmd) {
	if p == nil || !p.Open {
		return p, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	if p.ConfirmOpen {
		return p, p.updateConfirmation(key)
	}
	if p.MergeWarningOpen {
		return p, p.updateMergeWarning(key)
	}
	switch strings.ToLower(key.String()) {
	case "c":
		if p.Mode != ProjectRenameCatalogOnly {
			p.Mode = ProjectRenameCatalogOnly
			return p, p.invalidateOptions()
		}
	case "t":
		if p.CanSelectTaskMigration() {
			p.Mode = ProjectRenameCatalogAndPending
			return p, p.invalidateOptions()
		}
	case "s":
		if p.CanSelectSubprojects() {
			p.IncludeSubprojects = !p.IncludeSubprojects
			return p, p.invalidateOptions()
		}
	case "r":
		if !p.PreviewValid {
			return p, p.invalidateOptions()
		}
	case "enter":
		return p, p.requestConfirmation()
	case "esc", "escape":
		p.Close()
		return p, renameMessageCommand(ProjectRenameCancelMsg{})
	}
	return p, nil
}

func (p ProjectRenameModel) CanSelectTaskMigration() bool {
	return p.PreviewValid && p.Preview.ExactPlan.ValueChanged()
}

func (p ProjectRenameModel) CanSelectSubprojects() bool {
	// The same option also covers task-only descendants that are not present in
	// the configured catalog, so a configured child is not required to expose it.
	return p.Preview.ExactPlan.ValueChanged() && p.Preview.SubprojectsPlan.ValueChanged()
}

func (p *ProjectRenameModel) invalidateOptions() tea.Cmd {
	p.PreviewValid = false
	p.ConfirmOpen = false
	p.MergeWarningOpen = false
	p.Err = nil
	return renameMessageCommand(ProjectRenameOptionMsg{Mode: p.Mode, IncludeSubprojects: p.IncludeSubprojects})
}

func (p *ProjectRenameModel) requestConfirmation() tea.Cmd {
	if !p.PreviewValid {
		p.Err = fmt.Errorf("preview is stale; refresh it before confirming")
		return nil
	}
	if p.Preview.CollisionMessage != "" {
		p.Err = fmt.Errorf("%s", p.Preview.CollisionMessage)
		return nil
	}
	if p.Mode == ProjectRenameCatalogAndPending && p.Preview.DestinationTaskOnly {
		p.MergeWarningOpen = true
		return nil
	}
	p.ConfirmOpen = true
	return nil
}

func (p *ProjectRenameModel) updateMergeWarning(key tea.KeyPressMsg) tea.Cmd {
	switch strings.ToLower(key.String()) {
	case "enter", "y":
		p.MergeWarningOpen = false
		p.ConfirmOpen = true
	case "n", "esc", "escape":
		p.MergeWarningOpen = false
	}
	return nil
}

func (p *ProjectRenameModel) updateConfirmation(key tea.KeyPressMsg) tea.Cmd {
	switch strings.ToLower(key.String()) {
	case "y", "enter":
		message := ProjectRenameConfirmMsg{
			Preview:            cloneProjectRenamePreview(p.Preview),
			Plan:               cloneProjectRenamePlan(p.activePlan()),
			Mode:               p.Mode,
			IncludeSubprojects: p.IncludeSubprojects,
			TaskMappings:       append([]domain.ProjectTaskMapping(nil), p.activeTasks()...),
		}
		p.ConfirmOpen = false
		return renameMessageCommand(message)
	case "n", "esc", "escape":
		p.ConfirmOpen = false
		return renameMessageCommand(ProjectRenameCancelMsg{})
	}
	return nil
}

func (p ProjectRenameModel) activePlan() domain.ProjectCatalogPlan {
	if p.IncludeSubprojects && p.Preview.SubprojectsPlan.Kind != "" {
		return p.Preview.SubprojectsPlan
	}
	return p.Preview.ExactPlan
}

func (p ProjectRenameModel) activeTasks() []domain.ProjectTaskMapping {
	if p.IncludeSubprojects {
		return p.Preview.SubprojectTasks
	}
	return p.Preview.ExactTasks
}

func (p ProjectRenameModel) ViewAt(width, height int) string {
	p.SetSize(width, height)
	return p.View()
}

func (p ProjectRenameModel) View() string {
	if !p.Open || p.Width <= 0 || p.Height <= 0 {
		return ""
	}
	if p.MergeWarningOpen {
		return p.renderMergeWarning()
	}
	if p.ConfirmOpen {
		return p.renderConfirmation()
	}
	return p.renderPreview()
}

func (p ProjectRenameModel) renderPreview() string {
	plan := p.activePlan()
	lines := []string{p.Styles.ModalTitle.Render("Rename project")}
	if len(plan.Mappings) > 0 {
		for index, mapping := range plan.Mappings {
			prefix := ""
			if index > 0 {
				prefix = "  "
			}
			lines = append(lines, Truncate(prefix+mapping.OldValue+" → "+mapping.NewValue, ModalContentWidth(p.Width, ModalMaxWidth)))
		}
	}
	lines = append(lines,
		fmt.Sprintf("Catalog descendants: %d", p.catalogDescendantCount()),
		fmt.Sprintf("Task descendants: %d", p.taskDescendantCount()),
		fmt.Sprintf("Pending tasks: %d", len(p.activeTasks())),
	)
	if p.Preview.HasActiveContext {
		lines = append(lines, "Active context: "+p.Preview.ContextName)
	} else {
		lines = append(lines, "No active context (all eligible pending tasks)")
	}
	if plan.ValueChanged() {
		if p.Mode == ProjectRenameCatalogAndPending {
			lines = append(lines, "Mode: Catalog + pending tasks")
		} else {
			lines = append(lines, "Mode: Catalog only")
		}
		if p.CanSelectSubprojects() {
			choice := "off"
			if p.IncludeSubprojects {
				choice = "on"
			}
			lines = append(lines, "Include subprojects: "+choice+"  [s] toggle")
		}
		lines = append(lines, "[c] Catalog only   [t] Catalog + pending tasks")
	} else {
		lines = append(lines, "Name-only/no-op change: no task migration")
	}
	if p.Preview.DestinationTaskOnly && p.Mode == ProjectRenameCatalogAndPending {
		lines = append(lines, p.Styles.Error.Render("Destination is used by tasks but not in the catalog"))
	}
	if p.Preview.CollisionMessage != "" {
		lines = append(lines, p.Styles.Error.Render(Truncate(p.Preview.CollisionMessage, ModalContentWidth(p.Width, ModalMaxWidth))))
	}
	if !p.PreviewValid {
		lines = append(lines, p.Styles.Error.Render("Preview needs refresh before confirmation"))
	}
	lines = append(lines,
		p.Styles.Muted.Render("Historical tasks retain their values"),
		p.Styles.Muted.Render("native u cannot reverse this catalog/task operation"),
	)
	if p.Err != nil {
		lines = append(lines, p.Styles.Error.Render(Truncate(p.Err.Error(), ModalContentWidth(p.Width, ModalMaxWidth))))
	}
	lines = append(lines, p.Styles.ModalAction.Render("Enter confirm   r refresh   Esc cancel"))
	return p.renderPanel(lines)
}

func (p ProjectRenameModel) renderMergeWarning() string {
	lines := []string{
		p.Styles.ModalTitle.Render("Effective merge warning"),
		p.Styles.ModalBody.Render("The destination is used by Taskwarrior tasks but is not in the catalog."),
		p.Styles.ModalBody.Render("Migrating will combine task projects under one effective value."),
		p.Styles.ModalAction.Render("Enter confirm warning   Esc cancel"),
	}
	return p.renderPanel(lines)
}

func (p ProjectRenameModel) renderConfirmation() string {
	lines := []string{
		p.Styles.ModalTitle.Render("Confirm project operation"),
		fmt.Sprintf("Catalog descendants: %d", p.catalogDescendantCount()),
		fmt.Sprintf("Pending tasks: %d", len(p.activeTasks())),
		p.Styles.ModalAction.Render("[y] confirm   [n] cancel"),
	}
	return p.renderPanel(lines)
}

func (p ProjectRenameModel) catalogDescendantCount() int {
	plan := p.activePlan()
	if len(plan.Mappings) <= 1 {
		return 0
	}
	return len(plan.Mappings) - 1
}

func (p ProjectRenameModel) taskDescendantCount() int {
	plan := p.activePlan()
	if len(plan.Mappings) == 0 {
		return 0
	}
	root := plan.Mappings[0].OldValue
	count := 0
	for _, mapping := range p.activeTasks() {
		if strings.HasPrefix(mapping.OldValue, root+".") {
			count++
		}
	}
	return count
}

func (p ProjectRenameModel) renderPanel(lines []string) string {
	contentWidth := ModalContentWidth(p.Width, ModalMaxWidth)
	contentHeight := max(1, p.Height-2)
	return renderBoundedPanel(lines, contentWidth, contentHeight, p.Styles)
}

func renameMessageCommand(message tea.Msg) tea.Cmd {
	return func() tea.Msg { return message }
}

func cloneProjectRenamePreview(preview ProjectRenamePreview) ProjectRenamePreview {
	preview.ExactPlan = cloneProjectRenamePlan(preview.ExactPlan)
	preview.SubprojectsPlan = cloneProjectRenamePlan(preview.SubprojectsPlan)
	preview.ExactTasks = append([]domain.ProjectTaskMapping(nil), preview.ExactTasks...)
	preview.SubprojectTasks = append([]domain.ProjectTaskMapping(nil), preview.SubprojectTasks...)
	return preview
}

func cloneProjectRenamePlan(plan domain.ProjectCatalogPlan) domain.ProjectCatalogPlan {
	plan.Before = append(domain.ProjectCatalog(nil), plan.Before...)
	plan.After = append(domain.ProjectCatalog(nil), plan.After...)
	plan.Mappings = append([]domain.ProjectValueMapping(nil), plan.Mappings...)
	return plan
}
