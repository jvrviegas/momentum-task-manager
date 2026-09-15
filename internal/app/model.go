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
	ViewAuto  ViewName = "auto"
	ViewInbox ViewName = "inbox"
	ViewToday ViewName = "today"
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
	Client      taskwarrior.Client
	Config      config.Config
	InitialView ViewName
	Now         func() time.Time
	Context     context.Context
	Width       int
	Height      int
}

// Model is Momentum's root Bubble Tea state machine.
type Model struct {
	Client taskwarrior.Client
	Config config.Config
	ctx    context.Context
	now    func() time.Time

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
	Err           error

	MutationRunning bool
	PendingMutation *MutationRequest
	Sync            SyncState

	Styles       ui.Styles
	Icons        ui.Icons
	QuickAdd     ui.QuickAddModel
	Editor       ui.EditModel
	Details      ui.DetailsModel
	Confirm      ui.ConfirmModel
	Help         ui.HelpModel
	Quit         ui.QuitModel
	DeleteTarget string
}

// NewModel builds an application model with deterministic defaults.
func NewModel(options ModelOptions) *Model {
	settings := options.Config
	if settings == (config.Config{}) {
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
	styles := ui.NewStyles(ui.ResolveTheme(settings.Theme, true))
	icons := ui.IconsFor(settings.Icons)
	return &Model{
		Client:        options.Client,
		Config:        settings,
		ctx:           ctx,
		now:           now,
		RequestedView: requested,
		ActiveView:    normalizeView(requested),
		Selections:    map[ViewName]int{ViewInbox: 0, ViewToday: 0},
		Selected:      map[ViewName]string{ViewInbox: "", ViewToday: ""},
		Focus:         FocusList,
		Mode:          ModeLoading,
		Width:         options.Width,
		Height:        options.Height,
		Sync:          NewSyncState(settings.Sync, now()),
		Styles:        styles,
		Icons:         icons,
		QuickAdd:      ui.NewQuickAdd(styles, icons),
		Editor:        ui.NewEdit(styles, icons),
		Details:       ui.NewDetails(styles),
		Confirm:       ui.NewConfirm(styles),
		Help:          ui.NewHelp(styles),
		Quit:          ui.NewQuit(styles),
	}
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
	return LoadTasksCommand(m.ctx, m.Client, "initial")
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
		m.Editor.SetSize(message.Width, message.Height)
		m.Details.SetSize(message.Width, message.Height)
		m.Confirm.SetSize(message.Width, message.Height)
		m.Help.SetSize(message.Width, message.Height)
		m.Quit.SetSize(message.Width, message.Height)
		return m, nil
	case ui.QuickAddErrorMsg:
		m.QuickAdd.ApplyMessage(message)
		return m, nil
	case ui.QuickAddSubmitMsg:
		m.QuickAdd.ApplyMessage(message)
		m.Overlay = OverlayNone
		return m, m.beginMutation(MutationRequest{Kind: MutationAdd, Input: message.Task})
	case ui.EditErrorMsg:
		m.Editor.ApplyMessage(message)
		return m, nil
	case ui.EditSubmitMsg:
		return m, m.handleEditSubmit(message)
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
	default:
		return m, nil
	}
}

func (m *Model) applyTasks(message TasksMsg) tea.Cmd {
	oldTasks := len(m.Tasks)
	oldUUIDs := map[ViewName]string{
		ViewInbox: m.Selected[ViewInbox],
		ViewToday: m.Selected[ViewToday],
	}
	if message.Err != nil {
		m.Err = message.Err
		m.Status = "Refresh failed: " + conciseError(message.Err)
		if oldTasks == 0 {
			m.Mode = ModeError
		} else {
			m.Mode = ModeReady
		}
		return nil
	}
	m.Err = nil
	m.Tasks = append([]domain.Task(nil), message.Tasks...)
	m.Views = domain.BuildViews(m.Tasks, m.now())
	if m.RequestedView == ViewAuto {
		if len(m.Views.Today) > 0 {
			m.ActiveView = ViewToday
		} else {
			m.ActiveView = ViewInbox
		}
	} else {
		m.ActiveView = normalizeView(m.RequestedView)
	}
	m.restoreSelection(ViewInbox, oldUUIDs[ViewInbox])
	m.restoreSelection(ViewToday, oldUUIDs[ViewToday])
	m.Mode = ModeReady
	if message.Reason == "refresh" {
		m.Status = fmt.Sprintf("Updated %d tasks", len(m.Tasks))
	} else {
		m.Status = ""
	}
	return nil
}

func (m *Model) beginRefresh(reason string) tea.Cmd {
	if m.Mode == ModeLoading || m.Mode == ModeMutating || m.Overlay != OverlayNone {
		return nil
	}
	m.Mode = ModeRefreshing
	return LoadTasksCommand(m.ctx, m.Client, reason)
}

func (m *Model) beginMutation(request MutationRequest) tea.Cmd {
	if m.MutationRunning {
		return nil
	}
	m.MutationRunning = true
	m.Mode = ModeMutating
	m.PendingMutation = &request
	return MutationCommand(m.ctx, m.Client, request)
}

func (m *Model) applyMutation(message MutationMsg) tea.Cmd {
	pendingKind := MutationKind("")
	if m.PendingMutation != nil {
		pendingKind = m.PendingMutation.Kind
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
	m.Sync, _ = m.Sync.Mutation(m.now())
	kind := string(message.Kind)
	if kind == "" {
		kind = "action"
	}
	m.Status = strings.ToUpper(kind[:1]) + kind[1:] + " succeeded"
	// Every successful mutation has one and only one follow-up export. Sync
	// bookkeeping is attached by the feature-integration layer.
	return m.beginRefresh("mutation")
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
	tasks := m.tasksFor(view)
	previousIndex := m.Selections[view]
	index := domain.RestoreSelection(tasks, previousUUID, previousIndex)
	m.Selections[view] = index
	m.Selected[view] = domain.SelectedUUID(tasks, index)
}

func (m *Model) tasksFor(view ViewName) []domain.Task {
	if normalizeView(view) == ViewToday {
		return m.Views.Today
	}
	return m.Views.Inbox
}

func normalizeView(view ViewName) ViewName {
	if view == ViewToday {
		return ViewToday
	}
	return ViewInbox
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
		case OverlayEdit:
			_, cmd := m.Editor.Update(message)
			if !m.Editor.Open {
				m.Overlay = OverlayNone
			}
			return cmd
		case OverlayDetails:
			switch m.Details.Update(message) {
			case ui.DetailsEdit:
				m.Overlay = OverlayEdit
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
	switch message.String() {
	case "r":
		return m.beginRefresh("manual")
	case "ctrl+r":
		return m.beginSync()
	case "1":
		m.SwitchView(ViewInbox)
	case "2":
		m.SwitchView(ViewToday)
	case "j", "down":
		m.MoveSelection(1)
	case "k", "up":
		m.MoveSelection(-1)
	case "g":
		m.MoveSelection(-len(m.tasksFor(m.ActiveView)))
	case "G":
		m.MoveSelection(len(m.tasksFor(m.ActiveView)))
	case "ctrl+k":
		return m.OpenQuickAdd()
	case "/":
		m.Overlay = OverlaySearch
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
	m.ActiveView = view
	m.Focus = FocusList
	m.restoreSelection(view, m.Selected[view])
}

// MoveSelection moves within the active view and keeps the UUID identity.
func (m *Model) MoveSelection(delta int) {
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

// View implements tea.Model. Full composition is supplied by internal/ui in
// the later integration phase; this safe fallback is useful during loading.
func (m *Model) View() tea.View {
	if m == nil {
		return tea.NewView("")
	}
	return tea.NewView(m.Status)
}
