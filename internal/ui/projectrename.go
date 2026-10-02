package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
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
	Scroll             int
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
	p.Scroll = 0
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
	case "up", "k":
		p.Scroll = max(0, p.Scroll-1)
	case "down", "j":
		p.Scroll = min(p.Scroll+1, MaxOffset(len(p.previewRows()), p.Height-2))
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

func (p ProjectRenameModel) frameWidth() int {
	return min(RenameWidth, max(1, p.Width))
}

func (p ProjectRenameModel) renderPreview() string {
	frame := Frame{
		Title: "Rename project", Context: []Span{muted("nothing saved yet")}, Rows: p.previewRows(), Offset: p.Scroll,
		Width: p.frameWidth(), MaxRows: max(1, p.Height-2),
		Keys: []Hint{{"enter", "confirm"}, {"r", "refresh"}, {"esc", "cancel"}},
	}
	return strings.Join(p.Styles.RenderFrame(frame, Icons{}), "\n")
}

func (p ProjectRenameModel) previewRows() []FrameRow {
	icons := Icons{}.orUnicode()
	plan := p.activePlan()
	content := FrameContentWidth(p.frameWidth())
	rows := []FrameRow{{}}
	for _, mapping := range plan.Mappings {
		rows = append(rows, row(sp(mapping.OldValue, ToneCyan), muted(" "+icons.Arrow+" "), sp(mapping.NewValue, ToneCyan).bold()))
	}
	if len(plan.Mappings) > 0 {
		rows = append(rows, FrameRow{})
	}
	problems := len(rows)
	warning := func(tone Tone, text string) {
		for index, line := range WrapText(text, max(1, content-2)) {
			glyph := "  "
			if index == 0 {
				glyph = icons.Error + " "
			}
			rows = append(rows, row(sp(glyph, tone).bold(), sp(line, tone)))
		}
	}
	if p.Preview.DestinationTaskOnly && p.Mode == ProjectRenameCatalogAndPending {
		warning(ToneMedium, "Destination is used by tasks but not in the catalog")
	}
	if p.Preview.CollisionMessage != "" {
		warning(ToneRed, p.Preview.CollisionMessage)
	}
	if !p.PreviewValid {
		warning(ToneRed, "Preview needs refresh before confirmation")
	}
	if p.Err != nil {
		warning(ToneRed, p.Err.Error())
	}
	if len(rows) > problems {
		rows = append(rows, FrameRow{})
	}
	count := func(label string, value int) FrameRow {
		return row(muted(label+" "), txt(fmt.Sprintf("%d", value)).bold())
	}
	rows = append(rows,
		count("Catalog descendants:", p.catalogDescendantCount()),
		count("Task descendants:", p.taskDescendantCount()),
		count("Pending tasks:", len(p.activeTasks())),
	)
	if p.Preview.HasActiveContext {
		rows = append(rows, row(muted("Active context: "), txt(p.Preview.ContextName)))
	} else {
		rows = append(rows, row(muted("No active context (all eligible pending tasks)")))
	}
	rows = append(rows, FrameRow{})

	if plan.ValueChanged() {
		option := func(key, label string, on bool) FrameRow {
			r := row(txt(key).bold(), txt("  "), txt(label))
			if on {
				r.Mark = icons.Selection
				r.Spans[2] = txt(label).bold()
			}
			return r
		}
		rows = append(rows,
			option("c", "Catalog only", p.Mode != ProjectRenameCatalogAndPending),
			option("t", "Catalog + pending tasks", p.Mode == ProjectRenameCatalogAndPending))
		if p.CanSelectSubprojects() {
			choice := "off"
			if p.IncludeSubprojects {
				choice = "on"
			}
			rows = append(rows, row(txt("s").bold(), txt("  Include subprojects: "), txt(choice).bold()))
		}
	} else {
		rows = append(rows, row(muted("Name-only/no-op change: no task migration")))
	}
	rows = append(rows, FrameRow{},
		row(muted("Historical tasks retain their values")),
		row(muted("native u cannot reverse this catalog/task operation")))
	return append(rows, FrameRow{})
}

func (p ProjectRenameModel) renderMergeWarning() string {
	rows := []FrameRow{{}}
	for _, line := range WrapText("The destination is used by Taskwarrior tasks but is not in the catalog. Migrating will combine task projects under one effective value.", FrameContentWidth(p.frameWidth())) {
		rows = append(rows, row(txt(line)))
	}
	rows = append(rows, FrameRow{}, row(chip("enter", FillAccent), txt(" confirm warning"), gap(5), chip("esc", FillSelection), muted(" cancel")), FrameRow{})
	frame := Frame{Title: "Effective merge warning", Danger: true, Rows: fitRows(rows, max(1, p.Height-2)), Width: p.frameWidth(), MaxRows: max(1, p.Height-2)}
	return strings.Join(p.Styles.RenderFrame(frame, Icons{}), "\n")
}

func (p ProjectRenameModel) renderConfirmation() string {
	rows := []FrameRow{
		{},
		row(muted(fmt.Sprintf("%-22s", "Catalog descendants:")), txt(fmt.Sprintf("%d", p.catalogDescendantCount())).bold()),
		row(muted(fmt.Sprintf("%-22s", "Pending tasks:")), txt(fmt.Sprintf("%d", len(p.activeTasks()))).bold()),
		{},
		row(chip("y", FillAccent), txt(" confirm"), gap(5), chip("n", FillSelection), muted(" cancel")),
		{},
	}
	frame := Frame{Title: "Confirm project operation", Rows: fitRows(rows, max(1, p.Height-2)), Width: p.frameWidth(), MaxRows: max(1, p.Height-2)}
	return strings.Join(p.Styles.RenderFrame(frame, Icons{}), "\n")
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
