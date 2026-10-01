package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/config"
	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
	"github.com/jvrviegas/momentum/internal/ui"
)

// ViewName identifies one of Momentum's fixed views.
type ViewName string

const (
	ViewAuto      ViewName = "auto"
	ViewInbox     ViewName = "inbox"
	ViewToday     ViewName = "today"
	ViewCompleted ViewName = "completed"
	ViewSettings  ViewName = "settings"
)

// AppMode describes the root state independently from overlays.
type AppMode string

const (
	ModeLoading    AppMode = "loading"
	ModeReady      AppMode = "ready"
	ModeRefreshing AppMode = "refreshing"
	ModeMutating   AppMode = "mutating"
	ModeError      AppMode = "error"
)

// Overlay identifies the highest-precedence input surface.
type Overlay string

const (
	OverlayNone     Overlay = ""
	OverlayQuickAdd Overlay = "quick_add"
	OverlaySearch   Overlay = "search"
	OverlayEdit     Overlay = "edit"
	OverlayDetails  Overlay = "details"
	OverlayConfirm  Overlay = "confirm"
	OverlayHelp     Overlay = "help"
	OverlayQuit     Overlay = "quit"
	OverlayMinimum  Overlay = "minimum"
)

// FocusArea identifies sidebar/list focus in the root model.
type FocusArea string

const (
	FocusSidebar FocusArea = "sidebar"
	FocusList    FocusArea = "list"
)

// ModelOptions configures a root model without coupling tests to process
// globals. Client may be nil for rendering-only tests.
type ModelOptions struct {
	Client               taskwarrior.Client
	Config               config.Config
	InitialView          ViewName
	Now                  func() time.Time
	Context              context.Context
	Width                int
	Height               int
	DarkBackground       *bool
	ProjectStore         config.ProjectCatalogStore
	ProjectConfigPath    string
	MigrationCoordinator *ProjectMigrationCoordinator
}

// Model is Momentum's root Bubble Tea state machine.
type Model struct {
	Client            taskwarrior.Client
	Config            config.Config
	ProjectStore      config.ProjectCatalogStore
	ProjectConfigPath string
	ctx               context.Context
	now               func() time.Time

	Tasks []domain.Task
	Views domain.Views

	RequestedView ViewName
	ActiveView    ViewName
	Selections    map[ViewName]int
	Selected      map[ViewName]string
	Focus         FocusArea
	Overlay       Overlay
	Mode          AppMode
	Width         int
	Height        int
	Status        string
	TaskContext   string
	Err           error
	CompletedErr  error

	MutationRunning bool
	PendingMutation *MutationRequest
	Sync            SyncState
	SyncReady       bool
	SyncConfigured  bool

	Styles          ui.Styles
	Icons           ui.Icons
	QuickAdd        ui.QuickAddModel
	Search          ui.SearchModel
	Editor          ui.EditModel
	Details         ui.DetailsModel
	Confirm         ui.ConfirmModel
	Help            ui.HelpModel
	Quit            ui.QuitModel
	ProjectSettings ui.ProjectSettingsModel
	ProjectRename   ui.ProjectRenameModel

	DiscoveredProjects       []string
	ProjectDiscoveryID       uint64
	SettingsReturnView       ViewName
	ProjectSaveRunning       bool
	ProjectSaveID            uint64
	ProjectSnapshot          config.ProjectCatalogSnapshot
	MigrationCoordinator     *ProjectMigrationCoordinator
	RenamePreviewID          uint64
	RenamePreviewLoading     bool
	RenameExactPlan          domain.ProjectCatalogPlan
	RenameSubprojectsPlan    domain.ProjectCatalogPlan
	MigrationRunning         bool
	MigrationID              uint64
	PendingMigration         *ProjectMigrationRequest
	MigrationSyncBefore      SyncState
	MigrationRefreshPending  bool
	MigrationOutcome         *ProjectMigrationResult
	SyncDeferred             bool
	QuitAfterProjectSettings bool
	DeleteTarget             string
}

// NewModel builds an application model with deterministic defaults.
func NewModel(options ModelOptions) *Model {
	settings := options.Config
	if settings.IsZero() {
		settings = config.Defaults()
	}
	requested := options.InitialView
	if requested == "" {
		requested = ViewAuto
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	darkBackground := true
	if options.DarkBackground != nil {
		darkBackground = *options.DarkBackground
	}
	styles := ui.NewStyles(ui.ResolveTheme(settings.Theme, darkBackground))
	icons := ui.IconsFor(settings.Icons)
	syncReady := true
	syncConfigured := settings.Sync.Enabled && options.Client != nil
	if _, ok := options.Client.(interface {
		SyncConfigured(context.Context) (bool, error)
	}); ok {
		syncReady = false
		syncConfigured = false
	}
	model := &Model{
		Client:               options.Client,
		Config:               settings,
		ProjectStore:         options.ProjectStore,
		ProjectConfigPath:    options.ProjectConfigPath,
		MigrationCoordinator: options.MigrationCoordinator,
		ctx:                  ctx,
		now:                  now,
		RequestedView:        requested,
		ActiveView:           normalizeView(requested),
		Selections:           map[ViewName]int{ViewInbox: 0, ViewToday: 0, ViewCompleted: 0},
		Selected:             map[ViewName]string{ViewInbox: "", ViewToday: "", ViewCompleted: ""},
		Focus:                FocusList,
		Mode:                 ModeLoading,
		Width:                options.Width,
		Height:               options.Height,
		Sync:                 NewSyncState(settings.Sync, now()),
		SyncReady:            syncReady,
		SyncConfigured:       syncConfigured,
		Styles:               styles,
		Icons:                icons,
		QuickAdd:             ui.NewQuickAdd(styles, icons),
		Search:               ui.NewSearch(styles, icons),
		Editor:               ui.NewEdit(styles, icons),
		Details:              ui.NewDetails(styles),
		Confirm:              ui.NewConfirm(styles),
		Help:                 ui.NewHelp(styles),
		Quit:                 ui.NewQuit(styles),
		ProjectSettings:      ui.NewProjectSettings(styles),
		ProjectRename:        ui.NewProjectRename(styles),
	}
	model.Details.Icons = icons
	model.Confirm.Icons = icons
	model.Quit.Icons = icons
	model.ProjectSettings.SetProjects(settings.Projects)
	if model.ActiveView == ViewSettings {
		model.ProjectSettings.OpenProjects(settings.Projects)
	}
	model.QuickAdd.SetProjectCatalog(settings.Projects)
	model.Editor.SetProjectCatalog(settings.Projects)
	return model
}

// New is a concise constructor for application entry points.
func New(client taskwarrior.Client, settings config.Config, initial ViewName) *Model {
	return NewModel(ModelOptions{Client: client, Config: settings, InitialView: initial})
}

// Init starts local loading; it never performs a subprocess synchronously.
func (m *Model) Init() tea.Cmd {
	if m == nil {
		return nil
	}
	m.Mode = ModeLoading
	return LoadTasksCommand(m.ctx, m.Client, "initial", m.now())
}

// Update applies typed messages and delegates global input only when no overlay
// owns the keyboard.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m == nil {
		return m, nil
	}
	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = message.Width, message.Height
		m.QuickAdd.SetSize(message.Width, message.Height)
		m.Search.SetSize(message.Width, message.Height)
		m.Editor.SetSize(message.Width, message.Height)
		m.Details.SetSize(message.Width, message.Height)
		m.Confirm.SetSize(message.Width, message.Height)
		m.Help.SetSize(message.Width, message.Height)
		m.Quit.SetSize(message.Width, message.Height)
		m.ProjectSettings.SetSize(message.Width, message.Height)
		m.ProjectRename.SetSize(message.Width, message.Height)
		return m, nil
	case ui.QuickAddErrorMsg:
		m.QuickAdd.ApplyMessage(message)
		return m, nil
	case ui.QuickAddSubmitMsg:
		if m.Overlay != OverlayQuickAdd || !m.QuickAdd.Open {
			return m, nil
		}
		m.QuickAdd.ApplyMessage(message)
		m.Overlay = OverlayNone
		return m, m.beginMutation(MutationRequest{Kind: MutationAdd, Input: message.Task})
	case ui.EditErrorMsg:
		m.Editor.ApplyMessage(message)
		return m, nil
	case ui.EditSubmitMsg:
		if m.Overlay != OverlayEdit || !m.Editor.Open {
			return m, nil
		}
		return m, m.handleEditSubmit(message)
	case ui.ProjectSettingsSaveMsg:
		return m, m.handleProjectSettingsSave(message.Plan)
	case ProjectRenamePreviewMsg:
		return m, m.applyProjectRenamePreview(message)
	case ui.ProjectRenameOptionMsg:
		return m, m.refreshProjectRenamePreview(message)
	case ui.ProjectRenameConfirmMsg:
		return m, m.handleProjectRenameConfirm(message)
	case ui.ProjectRenameCancelMsg:
		m.RenamePreviewLoading = false
		m.ProjectRename.Close()
		return m, nil
	case ui.ProjectSettingsErrorMsg:
		m.ProjectSettings.ApplyMessage(message)
		return m, nil
	case ui.ProjectSettingsCancelMsg:
		if message.Close {
			m.leaveSettings()
		}
		return m, nil
	case ui.ProjectSettingsDiscardMsg:
		if m.QuitAfterProjectSettings {
			m.QuitAfterProjectSettings = false
			m.leaveSettings()
			return m, m.requestQuit()
		}
		m.Status = "Project changes discarded"
		return m, nil
	case ProjectCatalogSnapshotMsg:
		if message.Err == nil {
			m.ProjectSnapshot = message.Snapshot
		} else {
			m.Status = "Project config unavailable: " + conciseError(message.Err)
		}
		return m, nil
	case ProjectCatalogSaveMsg:
		return m, m.applyProjectCatalogSave(message)
	case ProjectMigrationMsg:
		return m, m.applyProjectMigration(message)
	case ui.SearchCommitMsg:
		m.Search.Query = message.Query
		m.Search.Active = message.Query != ""
		m.Overlay = OverlayNone
		m.restoreSelection(m.ActiveView, m.Selected[normalizeView(m.ActiveView)])
		return m, nil
	case ui.SearchClearMsg:
		m.Search.Clear()
		m.Overlay = OverlayNone
		m.restoreSelection(m.ActiveView, m.Selected[normalizeView(m.ActiveView)])
		return m, nil
	case ContextMsg:
		if message.Err == nil {
			m.TaskContext = strings.TrimSpace(message.Name)
		}
		return m, nil
	case ProjectsMsg:
		if message.RequestID != 0 && message.RequestID != m.ProjectDiscoveryID {
			return m, nil
		}
		if message.Err == nil {
			m.DiscoveredProjects = uniqueProjectValues(message.Values)
			m.refreshProjectSuggestions()
		}
		return m, nil
	case TagsMsg:
		if message.RequestID != 0 && message.RequestID != m.ProjectDiscoveryID {
			return m, nil
		}
		if message.Err == nil {
			m.QuickAdd.SetCatalog(m.QuickAdd.Projects, message.Values)
			m.Editor.SetCatalog(m.Editor.Projects, message.Values)
		}
		return m, nil
	case SyncConfigMsg:
		return m, m.applySyncConfig(message)
	case SyncMsg:
		return m, m.applySync(message)
	case SyncTickMsg:
		return m, m.handleSyncTick(time.Time(message))
	case RefreshTickMsg:
		return m, m.handleRefreshTick(time.Time(message))
	case TasksMsg:
		return m, m.applyTasks(message)
	case RefreshRequestedMsg:
		return m, m.beginRefresh(message.Reason)
	case MutationRequestedMsg:
		return m, m.beginMutation(message.Request)
	case MutationMsg:
		return m, m.applyMutation(message)
	case tea.KeyPressMsg:
		return m, m.updateKey(message)
	case tea.MouseClickMsg:
		return m, m.handleMouse(message)
	case tea.MouseWheelMsg:
		return m, m.handleMouse(message)
	default:
		return m, nil
	}
}

func (m *Model) applyTasks(message TasksMsg) tea.Cmd {
	oldTasks := len(m.Tasks)
	oldUUIDs := map[ViewName]string{
		ViewInbox:     m.Selected[ViewInbox],
		ViewToday:     m.Selected[ViewToday],
		ViewCompleted: m.Selected[ViewCompleted],
	}
	migrationRefresh := m.MigrationRunning && m.MigrationRefreshPending && message.Reason == "migration"
	if message.Err != nil {
		m.Err = message.Err
		m.Status = "Refresh failed: " + conciseError(message.Err)
		if oldTasks == 0 {
			m.Mode = ModeError
		} else {
			m.Mode = ModeReady
		}
		if migrationRefresh {
			m.finishProjectMigration()
		}
		if message.Reason == "initial" && m.Config.RefreshInterval > 0 {
			return refreshTickCommand(m.Config.RefreshInterval)
		}
		if migrationRefresh {
			return m.scheduleSync()
		}
		return nil
	}
	m.Err = nil
	m.Tasks = append([]domain.Task(nil), message.Tasks...)
	pendingViews := domain.BuildViews(m.Tasks, m.now())
	m.Views.Inbox = pendingViews.Inbox
	m.Views.Today = pendingViews.Today
	m.Views.Sections = pendingViews.Sections
	if message.CompletedLoaded {
		if message.CompletedErr == nil {
			m.CompletedErr = nil
			m.Views.Completed, m.Views.CompletedSections = domain.BuildCompletedView(message.Completed, m.now(), 30)
		} else {
			m.CompletedErr = message.CompletedErr
		}
	}
	// Initial-view preference chooses the startup view once. Later refreshes
	// must preserve the view the user navigated to, including behind overlays.
	if message.Reason == "initial" && m.ActiveView != ViewSettings {
		if m.RequestedView == ViewAuto {
			if len(m.Views.Today) > 0 {
				m.ActiveView = ViewToday
			} else {
				m.ActiveView = ViewInbox
			}
		} else {
			m.ActiveView = normalizeView(m.RequestedView)
		}
	}
	m.restoreSelection(ViewInbox, oldUUIDs[ViewInbox])
	m.restoreSelection(ViewToday, oldUUIDs[ViewToday])
	m.restoreSelection(ViewCompleted, oldUUIDs[ViewCompleted])
	m.Mode = ModeReady
	if migrationRefresh {
		m.finishProjectMigration()
	}
	if message.Reason == "migration" && m.MigrationOutcome != nil {
		result := m.MigrationOutcome
		m.Status = fmt.Sprintf("Migration: %d changed, %d skipped, %d failed, %d ambiguous; refreshed %d tasks", result.ChangedCount(), result.SkippedCount(), result.FailedCount(), result.AmbiguousCount(), len(m.Tasks))
	} else if message.Reason == "refresh" || message.Reason == "sync" || message.Reason == "mutation" || message.Reason == "migration" {
		m.Status = fmt.Sprintf("Updated %d tasks", len(m.Tasks))
	} else {
		m.Status = ""
	}
	if message.CompletedErr != nil {
		m.Status = "Completed refresh failed: " + conciseError(message.CompletedErr)
	}
	var commands []tea.Cmd
	if message.Reason == "initial" {
		if reader, ok := m.Client.(interface {
			Context(context.Context) (string, error)
		}); ok {
			commands = append(commands, ContextCommand(m.ctx, reader))
		}
		if !m.SyncReady {
			if reader, ok := m.Client.(interface {
				SyncConfigured(context.Context) (bool, error)
			}); ok {
				commands = append(commands, SyncConfigCommand(m.ctx, reader))
			}
		} else {
			commands = append(commands, m.startStartupSync())
		}
		if m.Config.RefreshInterval > 0 {
			commands = append(commands, refreshTickCommand(m.Config.RefreshInterval))
		}
		if m.ProjectStore != nil {
			commands = append(commands, ProjectCatalogSnapshotCommand(m.ctx, m.ProjectStore))
		}
	} else if m.SyncReady && m.Sync.NextAt.After(m.now()) && m.Sync.Phase != SyncInFlight {
		commands = append(commands, m.scheduleSync())
	}
	return tea.Batch(commands...)
}

func (m *Model) beginRefresh(reason string) tea.Cmd {
	if m.Mode == ModeLoading || m.Mode == ModeMutating || m.Overlay != OverlayNone || (m.MigrationRunning && reason != "migration") {
		return nil
	}
	m.Mode = ModeRefreshing
	return LoadTasksCommand(m.ctx, m.Client, reason, m.now())
}

func (m *Model) beginMutation(request MutationRequest) tea.Cmd {
	if m.MutationRunning || m.MigrationRunning {
		return nil
	}
	m.MutationRunning = true
	m.Mode = ModeMutating
	m.PendingMutation = &request
	return MutationCommand(m.ctx, m.Client, request)
}

func (m *Model) applyMutation(message MutationMsg) tea.Cmd {
	pendingKind := MutationKind("")
	var pending MutationRequest
	if m.PendingMutation != nil {
		pending = *m.PendingMutation
		pendingKind = pending.Kind
	}
	m.MutationRunning = false
	if message.Err != nil {
		if pendingKind == MutationModify {
			m.Editor.Err = message.Err
		}
		m.PendingMutation = nil
		m.Err = message.Err
		m.Mode = ModeReady
		m.Status = "Action failed: " + conciseError(message.Err)
		return nil
	}
	m.Err = nil
	m.Mode = ModeReady
	if pendingKind == MutationModify {
		m.Editor.Close()
		m.Overlay = OverlayNone
	}
	m.PendingMutation = nil
	if message.Kind == MutationUndo {
		m.Sync, _ = m.Sync.Apply(SyncEvent{Kind: SyncUndo, At: m.now()})
	} else {
		m.Sync, _ = m.Sync.Mutation(m.now())
	}
	m.Status = m.mutationStatus(message.Kind, pending)
	// Every successful mutation has one and only one follow-up export. Sync
	// bookkeeping is attached by the feature-integration layer.
	return m.beginRefresh("mutation")
}

// mutationStatus names what changed, e.g. Completed "Send Q3 invoice".
func (m *Model) mutationStatus(kind MutationKind, request MutationRequest) string {
	subject := request.Input.Description
	for _, task := range m.Tasks {
		if request.UUID != "" && task.UUID == request.UUID {
			subject = task.Description
			break
		}
	}
	quoted := ""
	if subject = oneLine(subject); subject != "" {
		quoted = fmt.Sprintf(" %q", subject)
	}
	switch kind {
	case MutationAdd:
		return "Added" + quoted
	case MutationModify:
		return "Saved" + quoted
	case MutationComplete:
		return "Completed" + quoted
	case MutationDelete:
		return "Deleted" + quoted
	case MutationStart:
		return "Started" + quoted
	case MutationStop:
		return "Stopped" + quoted
	case MutationUndo:
		return "Undid the last change"
	default:
		return "Done"
	}
}

func (m *Model) handleEditSubmit(message ui.EditSubmitMsg) tea.Cmd {
	if message.Diff.Empty() {
		m.Editor.Close()
		m.Overlay = OverlayNone
		m.Status = "No changes"
		return nil
	}
	uuid := m.Selected[normalizeView(m.ActiveView)]
	return m.beginMutation(MutationRequest{Kind: MutationModify, UUID: uuid, Diff: message.Diff})
}

func (m *Model) restoreSelection(view ViewName, previousUUID string) {
	view = normalizeView(view)
	if view == ViewSettings {
		return
	}
	tasks := m.tasksFor(view)
	previousIndex := m.Selections[view]
	index := domain.RestoreSelection(tasks, previousUUID, previousIndex)
	m.Selections[view] = index
	m.Selected[view] = domain.SelectedUUID(tasks, index)
}

func (m *Model) tasksFor(view ViewName) []domain.Task {
	view = normalizeView(view)
	if view == ViewSettings {
		return nil
	}
	var tasks []domain.Task
	switch view {
	case ViewToday:
		tasks = m.Views.Today
	case ViewCompleted:
		tasks = m.Views.Completed
	default:
		tasks = m.Views.Inbox
	}
	if m.Search.Active {
		return ui.FilterTasks(tasks, m.Search.Query)
	}
	return tasks
}

func normalizeView(view ViewName) ViewName {
	switch view {
	case ViewToday, ViewCompleted, ViewSettings:
		return view
	default:
		return ViewInbox
	}
}

func (m *Model) updateKey(message tea.KeyPressMsg) tea.Cmd {
	if m.Overlay != OverlayNone {
		switch m.Overlay {
		case OverlayQuickAdd:
			_, cmd := m.QuickAdd.Update(message)
			if !m.QuickAdd.Open {
				m.Overlay = OverlayNone
			}
			return cmd
		case OverlaySearch:
			_, cmd := m.Search.Update(message)
			if !m.Search.Open {
				m.Overlay = OverlayNone
			}
			return cmd
		case OverlayEdit:
			_, cmd := m.Editor.Update(message)
			if !m.Editor.Open {
				m.Overlay = OverlayNone
			}
			return cmd
		case OverlayDetails:
			if m.ActiveView == ViewCompleted && message.String() == "e" {
				m.Details.Close()
				m.Overlay = OverlayNone
				m.Status = "Completed tasks are read-only"
				return nil
			}
			switch m.Details.Update(message) {
			case ui.DetailsEdit:
				m.Overlay = OverlayEdit
				m.Editor.SetSize(m.Width, m.Height)
				return m.Editor.OpenTask(m.Details.Task, ui.FieldDescription)
			case ui.DetailsClose:
				m.Overlay = OverlayNone
			}
			return nil
		case OverlayConfirm:
			return m.ConfirmDelete(m.Confirm.Update(message))
		case OverlayHelp:
			if m.Help.Update(message) == ui.HelpClose {
				m.Overlay = OverlayNone
			}
			return nil
		case OverlayQuit:
			return m.handleQuitChoice(m.Quit.Update(message))
		default:
			return nil
		}
	}
	if m.ActiveView == ViewSettings {
		return m.updateSettingsKey(message)
	}
	switch message.String() {
	case "r":
		return m.beginRefresh("manual")
	case "ctrl+r":
		return m.beginSync()
	case "1":
		m.SwitchView(ViewInbox)
	case "2":
		m.SwitchView(ViewToday)
	case "3":
		m.SwitchView(ViewCompleted)
	case "4":
		m.SwitchView(ViewSettings)
	case "h":
		m.Focus = FocusSidebar
	case "l":
		m.Focus = FocusList
	case "tab":
		if m.Focus == FocusSidebar {
			m.Focus = FocusList
		} else {
			m.Focus = FocusSidebar
		}
	case "j", "down":
		m.MoveSelection(1)
	case "k", "up":
		m.MoveSelection(-1)
	case "g":
		m.MoveSelection(-len(m.tasksFor(m.ActiveView)))
	case "G":
		m.MoveSelection(len(m.tasksFor(m.ActiveView)))
	case "c", "ctrl+k":
		return m.OpenQuickAdd()
	case "/":
		return m.OpenSearch()
	case "esc", "escape":
		if m.Search.Active {
			m.Search.Clear()
			m.restoreSelection(m.ActiveView, m.Selected[normalizeView(m.ActiveView)])
		}
	case "enter":
		m.OpenDetails()
	case " ", "space":
		return m.CompleteSelected()
	case "e":
		return m.OpenEditor(ui.FieldDescription)
	case "p":
		return m.OpenEditor(ui.FieldProject)
	case "!":
		return m.OpenEditor(ui.FieldPriority)
	case "D":
		return m.OpenEditor(ui.FieldDue)
	case "S":
		return m.OpenEditor(ui.FieldScheduled)
	case "t":
		return m.OpenEditor(ui.FieldTags)
	case "s":
		return m.ToggleStartSelected()
	case "d":
		m.DeleteSelected()
	case "u":
		return m.UndoLast()
	case "?":
		m.OpenHelp()
	case "q", "ctrl+c":
		return m.requestQuit()
	}
	return nil
}

// SwitchView changes the active fixed view and restores its UUID selection.
func (m *Model) SwitchView(view ViewName) {
	view = normalizeView(view)
	if view == ViewSettings {
		if m.ActiveView != ViewSettings {
			m.SettingsReturnView = normalizeTaskView(m.ActiveView)
		}
		m.ActiveView = ViewSettings
		m.Focus = FocusList
		m.ProjectSettings.SetSize(m.Width, m.Height)
		if !m.ProjectSettings.Open {
			m.ProjectSettings.OpenProjects(m.Config.Projects)
		}
		return
	}
	if m.ActiveView == ViewSettings {
		if m.ProjectSaveRunning || m.ProjectSettings.Dirty() {
			m.Status = "Save or discard project changes before leaving Settings"
			return
		}
		m.ProjectSettings.Close()
	}
	m.ActiveView = view
	m.Focus = FocusList
	m.restoreSelection(view, m.Selected[view])
}

// MoveSelection moves within the active view and keeps the UUID identity.
func (m *Model) MoveSelection(delta int) {
	if m.ActiveView == ViewSettings {
		return
	}
	view := normalizeView(m.ActiveView)
	m.ActiveView = view
	tasks := m.tasksFor(view)
	if len(tasks) == 0 {
		m.Selections[view] = -1
		m.Selected[view] = ""
		return
	}
	index := m.Selections[view]
	if index < 0 {
		index = 0
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(tasks) {
		index = len(tasks) - 1
	}
	m.Selections[view] = index
	m.Selected[view] = tasks[index].UUID
}

func quitCommand() tea.Cmd {
	return func() tea.Msg { return tea.Quit() }
}

func conciseError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if len(text) > 160 {
		return text[:157] + "..."
	}
	return text
}
