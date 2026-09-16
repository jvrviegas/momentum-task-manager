package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum/internal/domain"
)

// ProjectSettingsField identifies an editable catalog draft field.
type ProjectSettingsField int

const (
	ProjectSettingsName ProjectSettingsField = iota
	ProjectSettingsValue
	ProjectSettingsParent
)

var projectSettingsFieldNames = [...]string{"Name", "Value", "Parent"}

// ProjectSettingsSaveMsg is the explicit catalog-save intent sent to the app.
type ProjectSettingsSaveMsg struct {
	Plan domain.ProjectCatalogPlan
}

// ProjectSettingsSavedMsg reports the result of a save started by the app.
// Projects is optional; when nil, Plan.After is used.
type ProjectSettingsSavedMsg struct {
	Plan     domain.ProjectCatalogPlan
	Projects domain.ProjectCatalog
	Err      error
}

// ProjectSettingsErrorMsg keeps a validation or local planning error in the
// editor without discarding its draft.
type ProjectSettingsErrorMsg struct{ Err error }

// ProjectSettingsCancelMsg is emitted when the current edit or settings screen
// is explicitly canceled without changing the catalog.
type ProjectSettingsCancelMsg struct {
	Close bool
}

// ProjectSettingsDiscardMsg is emitted after the user explicitly discards a
// dirty draft.
type ProjectSettingsDiscardMsg struct{}

// ProjectSettingsModel is a pure editor for Momentum's configured project
// catalog. It owns draft state and emits intents; it performs no I/O.
type ProjectSettingsModel struct {
	Projects  domain.ProjectCatalog
	Selected  int
	Open      bool
	Editing   bool
	EditIndex int
	Focused   ProjectSettingsField
	Inputs    [3]textinput.Model

	Suggestions     []string
	SuggestionIndex int
	SuggestionsOpen bool

	RemovePromptOpen  bool
	RemoveTargetValue string
	RemoveChildCount  int
	DiscardPromptOpen bool

	Width  int
	Height int
	Styles Styles
	Err    error
}

// NewProjectSettings creates a closed editor with initialized text inputs.
func NewProjectSettings(styles Styles) ProjectSettingsModel {
	model := ProjectSettingsModel{Styles: styles, EditIndex: -1}
	for i := range model.Inputs {
		model.Inputs[i] = textinput.New()
		model.Inputs[i].Prompt = ""
		model.Inputs[i].Placeholder = projectSettingsFieldNames[i]
		model.Inputs[i].Blur()
	}
	return model
}

// SetSize updates the component's responsive dimensions without doing I/O.
func (p *ProjectSettingsModel) SetSize(width, height int) {
	p.Width, p.Height = width, height
	for i := range p.Inputs {
		p.Inputs[i].SetWidth(max(1, width-22))
	}
}

// OpenProjects opens the list with an owned catalog snapshot.
func (p *ProjectSettingsModel) OpenProjects(projects domain.ProjectCatalog) {
	p.Projects = cloneProjectSettingsCatalog(projects)
	p.Open = true
	p.Editing = false
	p.EditIndex = -1
	p.RemovePromptOpen = false
	p.DiscardPromptOpen = false
	p.Err = nil
	p.clampSelection()
	p.clearInputs()
}

// SetProjects replaces the displayed catalog when no dirty draft is active.
func (p *ProjectSettingsModel) SetProjects(projects domain.ProjectCatalog) {
	if p.Editing && p.Dirty() {
		return
	}
	p.Projects = cloneProjectSettingsCatalog(projects)
	p.clampSelection()
}

// Close closes the settings component and clears transient draft state.
func (p *ProjectSettingsModel) Close() {
	p.Open = false
	p.Editing = false
	p.RemovePromptOpen = false
	p.DiscardPromptOpen = false
	p.Err = nil
	p.clearInputs()
}

// OpenAdd starts a new catalog draft.
func (p *ProjectSettingsModel) OpenAdd() tea.Cmd {
	p.Open = true
	p.Editing = true
	p.EditIndex = -1
	p.Err = nil
	p.RemovePromptOpen = false
	p.DiscardPromptOpen = false
	p.clearInputs()
	return p.focus(ProjectSettingsName)
}

// OpenEdit starts a draft for the exact configured entry at index.
func (p *ProjectSettingsModel) OpenEdit(index int) tea.Cmd {
	if index < 0 || index >= len(p.Projects) {
		return nil
	}
	p.Open = true
	p.Editing = true
	p.EditIndex = index
	p.Err = nil
	p.RemovePromptOpen = false
	p.DiscardPromptOpen = false
	draft := domain.DraftForProject(p.Projects[index])
	p.Inputs[ProjectSettingsName].SetValue(draft.Name)
	p.Inputs[ProjectSettingsValue].SetValue(draft.ValueSegment)
	p.Inputs[ProjectSettingsParent].SetValue(draft.ParentValue)
	for i := range p.Inputs {
		p.Inputs[i].Blur()
	}
	return p.focus(ProjectSettingsName)
}

// Input returns a mutable input for tests and composition code.
func (p *ProjectSettingsModel) Input(field ProjectSettingsField) *textinput.Model {
	if !validProjectSettingsField(field) {
		return nil
	}
	return &p.Inputs[field]
}

// CurrentDraft returns the current form values.
func (p ProjectSettingsModel) CurrentDraft() domain.ProjectDraft {
	return domain.ProjectDraft{
		Name:         p.Inputs[ProjectSettingsName].Value(),
		ValueSegment: p.Inputs[ProjectSettingsValue].Value(),
		ParentValue:  p.Inputs[ProjectSettingsParent].Value(),
	}
}

// Dirty reports whether the current draft differs from the draft at open.
func (p ProjectSettingsModel) Dirty() bool {
	if !p.Editing {
		return false
	}
	return p.CurrentDraft() != p.initialDraft()
}

// PromptDiscard opens the explicit dirty-draft prompt and reports whether it
// was opened.
func (p *ProjectSettingsModel) PromptDiscard() bool {
	if p == nil || !p.Dirty() {
		return false
	}
	p.DiscardPromptOpen = true
	p.Err = nil
	return true
}

// PreviewValue composes the full dotted value currently entered.
func (p ProjectSettingsModel) PreviewValue() (string, error) {
	draft := p.CurrentDraft()
	return domain.ComposeProjectValue(draft.ParentValue, draft.ValueSegment)
}

// Update handles list navigation, draft editing, and explicit prompts.
func (p *ProjectSettingsModel) Update(msg tea.Msg) (*ProjectSettingsModel, tea.Cmd) {
	if p == nil || !p.Open {
		return p, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	if p.RemovePromptOpen {
		return p, p.updateRemovePrompt(key)
	}
	if p.DiscardPromptOpen {
		return p, p.updateDiscardPrompt(key)
	}
	if p.Editing {
		return p, p.updateEditor(key)
	}
	return p, p.updateList(key)
}

// ApplyMessage applies a typed save or validation result from the app.
func (p *ProjectSettingsModel) ApplyMessage(msg tea.Msg) (tea.Msg, bool) {
	switch message := msg.(type) {
	case ProjectSettingsErrorMsg:
		p.Err = message.Err
		return nil, true
	case ProjectSettingsSavedMsg:
		if message.Err != nil {
			p.Err = message.Err
			return nil, true
		}
		projects := message.Projects
		if projects == nil {
			projects = message.Plan.After
		}
		p.Projects = cloneProjectSettingsCatalog(projects)
		p.Err = nil
		p.Editing = false
		p.EditIndex = -1
		p.RemovePromptOpen = false
		p.DiscardPromptOpen = false
		p.clearInputs()
		p.clampSelection()
		return nil, true
	default:
		return nil, false
	}
}

func (p *ProjectSettingsModel) updateList(key tea.KeyPressMsg) tea.Cmd {
	switch strings.ToLower(key.String()) {
	case "up", "k":
		p.moveSelection(-1)
	case "down", "j":
		p.moveSelection(1)
	case "a":
		return p.OpenAdd()
	case "e", "enter":
		return p.OpenEdit(p.Selected)
	case "d":
		return p.beginRemove()
	case "esc", "escape", "q":
		p.Close()
		return settingsMessageCommand(ProjectSettingsCancelMsg{Close: true})
	}
	return nil
}

func (p *ProjectSettingsModel) updateEditor(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "esc", "escape", "q":
		if p.Dirty() {
			p.DiscardPromptOpen = true
			p.Err = nil
			return nil
		}
		p.Editing = false
		p.clearInputs()
		p.Err = nil
		return settingsMessageCommand(ProjectSettingsCancelMsg{Close: false})
	case "tab":
		if p.SuggestionsOpen {
			p.acceptSuggestion()
			return nil
		}
		p.moveField(1)
		return nil
	case "shift+tab":
		p.moveField(-1)
		return nil
	case "up", "ctrl+p":
		if p.SuggestionsOpen {
			p.moveSuggestion(-1)
		} else {
			p.moveField(-1)
		}
		return nil
	case "down", "ctrl+n":
		if p.SuggestionsOpen {
			p.moveSuggestion(1)
		} else {
			p.moveField(1)
		}
		return nil
	case "enter":
		if p.SuggestionsOpen {
			p.acceptSuggestion()
			return nil
		}
		return p.submit()
	case "ctrl+s":
		return p.submit()
	}
	var cmd tea.Cmd
	p.Inputs[p.Focused], cmd = p.Inputs[p.Focused].Update(key)
	p.Err = nil
	p.refreshSuggestions()
	return cmd
}

func (p *ProjectSettingsModel) updateRemovePrompt(key tea.KeyPressMsg) tea.Cmd {
	switch strings.ToLower(key.String()) {
	case "n":
		p.RemovePromptOpen = false
		return p.remove(false)
	case "y":
		p.RemovePromptOpen = false
		return p.remove(true)
	case "esc", "escape":
		p.RemovePromptOpen = false
		p.RemoveTargetValue = ""
		p.RemoveChildCount = 0
	}
	return nil
}

func (p *ProjectSettingsModel) updateDiscardPrompt(key tea.KeyPressMsg) tea.Cmd {
	switch strings.ToLower(key.String()) {
	case "s":
		p.DiscardPromptOpen = false
		return p.submit()
	case "d":
		p.DiscardPromptOpen = false
		p.Editing = false
		p.EditIndex = -1
		p.clearInputs()
		p.Err = nil
		return settingsMessageCommand(ProjectSettingsDiscardMsg{})
	case "esc", "escape":
		p.DiscardPromptOpen = false
	}
	return nil
}

func (p *ProjectSettingsModel) beginRemove() tea.Cmd {
	project, ok := p.selectedProject()
	if !ok {
		return nil
	}
	p.RemoveTargetValue = project.Value
	children := 0
	for _, candidate := range p.Projects {
		if strings.HasPrefix(candidate.Value, project.Value+".") {
			children++
		}
	}
	if children > 0 {
		p.RemovePromptOpen = true
		p.RemoveTargetValue = project.Value
		p.RemoveChildCount = children
		p.Err = nil
		return nil
	}
	return p.remove(false)
}

func (p *ProjectSettingsModel) remove(removeChildren bool) tea.Cmd {
	plan, err := domain.PlanRemoveProject(p.Projects, p.RemoveTargetValue, removeChildren)
	if err != nil {
		p.Err = err
		return settingsMessageCommand(ProjectSettingsErrorMsg{Err: err})
	}
	p.RemoveTargetValue = ""
	p.RemoveChildCount = 0
	return settingsMessageCommand(ProjectSettingsSaveMsg{Plan: plan})
}

func (p *ProjectSettingsModel) submit() tea.Cmd {
	draft := p.CurrentDraft()
	var (
		plan domain.ProjectCatalogPlan
		err  error
	)
	if p.EditIndex < 0 {
		plan, err = domain.PlanAddDraft(p.Projects, draft)
	} else if p.EditIndex < len(p.Projects) {
		plan, err = domain.PlanUpdateDraft(p.Projects, p.Projects[p.EditIndex].Value, draft, false)
	} else {
		err = fmt.Errorf("project edit selection is no longer available")
	}
	if err != nil {
		return settingsMessageCommand(ProjectSettingsErrorMsg{Err: err})
	}
	return settingsMessageCommand(ProjectSettingsSaveMsg{Plan: cloneProjectSettingsPlan(plan)})
}

func (p *ProjectSettingsModel) initialDraft() domain.ProjectDraft {
	if p.EditIndex < 0 || p.EditIndex >= len(p.Projects) {
		return domain.ProjectDraft{}
	}
	return domain.DraftForProject(p.Projects[p.EditIndex])
}

func (p *ProjectSettingsModel) focus(field ProjectSettingsField) tea.Cmd {
	if !validProjectSettingsField(field) {
		return nil
	}
	for i := range p.Inputs {
		if ProjectSettingsField(i) == field {
			continue
		}
		p.Inputs[i].Blur()
	}
	p.Focused = field
	cmd := p.Inputs[field].Focus()
	p.refreshSuggestions()
	return cmd
}

func (p *ProjectSettingsModel) moveField(delta int) {
	field := int(p.Focused) + delta
	if field < 0 {
		field = len(p.Inputs) - 1
	}
	if field >= len(p.Inputs) {
		field = 0
	}
	p.SuggestionsOpen = false
	p.Suggestions = nil
	p.focus(ProjectSettingsField(field))
}

func (p *ProjectSettingsModel) refreshSuggestions() {
	if !p.Editing || p.Focused != ProjectSettingsParent {
		p.SuggestionsOpen = false
		p.Suggestions = nil
		return
	}
	query := strings.TrimSpace(p.Inputs[ProjectSettingsParent].Value())
	current := ""
	if p.EditIndex >= 0 && p.EditIndex < len(p.Projects) {
		current = p.Projects[p.EditIndex].Value
	}
	labels := p.Projects.Labels()
	p.Suggestions = p.Suggestions[:0]
	seen := map[string]struct{}{}
	for _, project := range p.Projects {
		if project.Value == current || strings.HasPrefix(project.Value, current+".") {
			continue
		}
		if query != "" && !fuzzyContains(query, project.Value) && !fuzzyContains(query, labels[project.Value]) {
			continue
		}
		if _, ok := seen[project.Value]; ok {
			continue
		}
		seen[project.Value] = struct{}{}
		p.Suggestions = append(p.Suggestions, project.Value)
	}
	p.SuggestionsOpen = len(p.Suggestions) > 0
	if p.SuggestionIndex >= len(p.Suggestions) {
		p.SuggestionIndex = 0
	}
}

func (p *ProjectSettingsModel) moveSuggestion(delta int) {
	if len(p.Suggestions) == 0 {
		return
	}
	p.SuggestionIndex = (p.SuggestionIndex + delta) % len(p.Suggestions)
	if p.SuggestionIndex < 0 {
		p.SuggestionIndex += len(p.Suggestions)
	}
}

func (p *ProjectSettingsModel) acceptSuggestion() {
	if len(p.Suggestions) == 0 {
		return
	}
	p.Inputs[ProjectSettingsParent].SetValue(p.Suggestions[p.SuggestionIndex])
	p.Inputs[ProjectSettingsParent].CursorEnd()
	p.refreshSuggestions()
}

func (p *ProjectSettingsModel) moveSelection(delta int) {
	if len(p.Projects) == 0 {
		p.Selected = 0
		return
	}
	p.Selected += delta
	p.clampSelection()
}

func (p *ProjectSettingsModel) clampSelection() {
	if len(p.Projects) == 0 {
		p.Selected = 0
		return
	}
	if p.Selected < 0 {
		p.Selected = 0
	}
	if p.Selected >= len(p.Projects) {
		p.Selected = len(p.Projects) - 1
	}
}

func (p *ProjectSettingsModel) clearInputs() {
	for i := range p.Inputs {
		p.Inputs[i].SetValue("")
		p.Inputs[i].Blur()
	}
	p.Suggestions = nil
	p.SuggestionIndex = 0
	p.SuggestionsOpen = false
}

func (p *ProjectSettingsModel) selectedProject() (domain.Project, bool) {
	if p.Selected < 0 || p.Selected >= len(p.Projects) {
		return domain.Project{}, false
	}
	return p.Projects[p.Selected], true
}

// ViewAt renders a settings snapshot at a composition-specific size without
// mutating the live component.
func (p ProjectSettingsModel) ViewAt(width, height int) string {
	p.SetSize(width, height)
	return p.View()
}

// View renders the settings list, draft form, or explicit prompt.
func (p ProjectSettingsModel) View() string {
	if !p.Open || p.Width <= 0 || p.Height <= 0 {
		return ""
	}
	switch {
	case p.RemovePromptOpen:
		return p.renderRemovePrompt()
	case p.DiscardPromptOpen:
		return p.renderDiscardPrompt()
	case p.Editing:
		return p.renderEditor()
	default:
		return p.renderList()
	}
}

func (p ProjectSettingsModel) renderList() string {
	lines := []string{
		p.Styles.Title.Render("Settings / Projects"),
		p.Styles.Muted.Render("[a] add   [e] edit   [d] remove   [Enter] edit"),
	}
	if len(p.Projects) == 0 {
		lines = append(lines,
			p.Styles.Muted.Render("No configured projects"),
			p.Styles.Muted.Render("Press a to add a project."),
		)
	} else {
		labels := p.Projects.Labels()
		visible := max(1, p.Height-5)
		start := p.Selected - visible + 1
		if start < 0 {
			start = 0
		}
		end := min(len(p.Projects), start+visible)
		for i := start; i < end; i++ {
			project := p.Projects[i]
			label := labels[project.Value]
			if label == "" {
				label = project.Name
			}
			if label == "" {
				label = project.Value
			}
			line := fmt.Sprintf("%s %s  (%s)", marker(i == p.Selected), label, project.Value)
			line = Truncate(line, max(1, p.Width-4))
			if i == p.Selected {
				line = p.Styles.Selection.Render(line)
			}
			lines = append(lines, line)
		}
	}
	if p.Err != nil {
		lines = append(lines, p.Styles.Overdue.Render(Truncate(p.Err.Error(), max(1, p.Width-4))))
	}
	lines = append(lines, p.Styles.Muted.Render("Esc to close"))
	return p.renderPanel(lines)
}

func (p ProjectSettingsModel) renderEditor() string {
	title := "Add project"
	if p.EditIndex >= 0 {
		title = "Edit project"
	}
	lines := []string{
		p.Styles.Title.Render(title),
		p.renderField(ProjectSettingsName),
		p.renderField(ProjectSettingsValue),
		p.renderField(ProjectSettingsParent),
	}
	preview, err := p.PreviewValue()
	if err != nil {
		lines = append(lines, p.Styles.Overdue.Render("Full value: "+Truncate(err.Error(), max(1, p.Width-15))))
	} else {
		lines = append(lines, p.Styles.Project.Render("Full value: "+Truncate(preview, max(1, p.Width-15))))
	}
	if p.SuggestionsOpen {
		values := append([]string(nil), p.Suggestions...)
		if len(values) > 0 {
			values[p.SuggestionIndex] = "▸ " + values[p.SuggestionIndex]
		}
		lines = append(lines, p.Styles.Muted.Render(Truncate("Parents: "+strings.Join(values, "  "), max(1, p.Width-4))))
	}
	if p.Err != nil {
		lines = append(lines, p.Styles.Overdue.Render(Truncate(p.Err.Error(), max(1, p.Width-4))))
	}
	lines = append(lines, p.Styles.Muted.Render("Tab next   Ctrl+S save   Esc cancel"))
	return p.renderPanel(lines)
}

func (p ProjectSettingsModel) renderField(field ProjectSettingsField) string {
	marker := " "
	if p.Focused == field {
		marker = "▸"
	}
	return fmt.Sprintf("%s %-7s %s", marker, projectSettingsFieldNames[field], p.Inputs[field].View())
}

func (p ProjectSettingsModel) renderRemovePrompt() string {
	project := p.RemoveTargetValue
	lines := []string{
		p.Styles.Title.Render("Remove project?"),
		Truncate(project, max(1, p.Width-4)),
		fmt.Sprintf("Configured children: %d", p.RemoveChildCount),
		"[n] only this entry",
		"[y] this entry and its children",
		"[Esc] cancel",
	}
	return p.renderPanel(lines)
}

func (p ProjectSettingsModel) renderDiscardPrompt() string {
	lines := []string{
		p.Styles.Title.Render("Unsaved project changes"),
		"[s] save   [d] discard   [Esc] cancel",
	}
	return p.renderPanel(lines)
}

func (p ProjectSettingsModel) renderPanel(lines []string) string {
	maxLines := max(1, p.Height-2)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return p.Styles.Border.Width(max(1, p.Width-2)).Render(body)
}

func marker(selected bool) string {
	if selected {
		return "▸"
	}
	return " "
}

func settingsMessageCommand(message tea.Msg) tea.Cmd {
	return func() tea.Msg { return message }
}

func validProjectSettingsField(field ProjectSettingsField) bool {
	return field >= ProjectSettingsName && field <= ProjectSettingsParent
}

func cloneProjectSettingsCatalog(projects domain.ProjectCatalog) domain.ProjectCatalog {
	if projects == nil {
		return nil
	}
	return append(domain.ProjectCatalog(nil), projects...)
}

func cloneProjectSettingsPlan(plan domain.ProjectCatalogPlan) domain.ProjectCatalogPlan {
	plan.Before = cloneProjectSettingsCatalog(plan.Before)
	plan.After = cloneProjectSettingsCatalog(plan.After)
	plan.Mappings = append([]domain.ProjectValueMapping(nil), plan.Mappings...)
	return plan
}
