package app

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/calendar"
	"github.com/jvrviegas/momentum/internal/config"
	"github.com/jvrviegas/momentum/internal/domain"
	"github.com/jvrviegas/momentum/internal/ui"
)

// CalendarState is an ephemeral planning projection. Event descriptions are
// held only while the planner is open; they are never persisted, logged, or
// written to Taskwarrior data.
type CalendarState struct {
	Available    bool
	Stale        bool
	BusyMinutes  int
	MeetingCount int
	Events       []ui.PlannerEvent
	Err          string
}

// OpenPlanner opens the task-only ritual immediately, then optionally loads
// configured local ICS sources without blocking the Bubble Tea update loop.
func (m *Model) OpenPlanner() tea.Cmd {
	if m == nil || m.MutationRunning || m.ActiveView == ViewSettings || !m.Config.Planning.Enabled {
		return nil
	}
	m.CalendarResult = CalendarState{}
	m.refreshDailyPlanView()
	m.Planner.SetSize(m.Width, m.Height)
	m.Planner.OpenPlan(m.DailyPlan.Date, plannerItems(m.DailyPlan), plannerSummary(m.DailyPlan.Capacity, m.CalendarResult))
	m.Overlay = OverlayPlanner
	if !m.Config.Calendar.Enabled {
		return nil
	}
	m.CalendarID++
	return CalendarCommand(m.ctx, m.CalendarID, m.Config.Calendar, m.now())
}

func (m *Model) refreshDailyPlanView() {
	if m == nil {
		return
	}
	m.DailyPlan = domain.BuildDailyPlan(m.Tasks, m.nowTime(), planningConstraints(m.Config, m.CalendarResult, m.nowTime()))
	if m.Planner.Open {
		m.Planner.OpenPlan(m.DailyPlan.Date, plannerItems(m.DailyPlan), plannerSummary(m.DailyPlan.Capacity, m.CalendarResult))
		m.Planner.SetSize(m.Width, m.Height)
	}
}

func (m *Model) handlePlannerSubmit(uuids []string) tea.Cmd {
	if m.MutationRunning || !m.Planner.Open {
		return nil
	}
	selected := make(map[string]bool, len(uuids))
	for _, uuid := range uuids {
		if strings.TrimSpace(uuid) != "" {
			selected[uuid] = true
		}
	}
	mutations := domain.BuildPlanMutations(m.Tasks, selected, m.nowTime())
	if len(mutations) == 0 {
		m.Planner.Close()
		m.Overlay = OverlayNone
		m.Status = "Daily plan unchanged"
		return nil
	}
	return m.beginMutation(MutationRequest{Kind: MutationPlan, PlanMutations: mutations})
}

func (m *Model) applyCalendar(message CalendarMsg) tea.Cmd {
	if message.ID != 0 && message.ID != m.CalendarID {
		return nil
	}
	if !m.Planner.Open || m.Overlay != OverlayPlanner {
		return nil
	}
	m.CalendarResult = message.Result
	if m.Planner.Open {
		selected := make(map[string]bool)
		for _, uuid := range m.Planner.SelectedUUIDs() {
			selected[uuid] = true
		}
		m.DailyPlan = domain.BuildDailyPlan(m.Tasks, m.nowTime(), planningConstraints(m.Config, m.CalendarResult, m.nowTime()))
		m.Planner.Summary = plannerSummary(m.DailyPlan.Capacity, m.CalendarResult)
		for index := range m.Planner.Items {
			m.Planner.Items[index].Selected = selected[m.Planner.Items[index].UUID]
		}
		m.recalculatePlannerSummaryFromUI()
	}
	return nil
}

// CalendarCommand reads local ICS sources and returns only busy-minute/count
// summaries. It is safe to run while the planner remains interactive.
func CalendarCommand(ctx context.Context, id uint64, settings config.CalendarConfig, now time.Time) tea.Cmd {
	return func() tea.Msg {
		home, _ := os.UserHomeDir()
		result := calendar.Load(ctx, calendar.Options{
			Paths:              settings.PathsFor(home),
			IncludeAllDay:      settings.IncludeAllDay,
			IncludeTransparent: settings.IncludeTransparent,
			WorkingStart:       settings.WorkingStart,
			WorkingEnd:         settings.WorkingEnd,
			StaleAfter:         settings.StaleAfter,
			Now:                now,
		})
		state := CalendarState{Stale: result.Stale}
		if result.Err != nil {
			state.Err = conciseError(result.Err)
			return CalendarMsg{ID: id, Result: state}
		}
		intervals, err := calendar.BusyIntervalsWithOptions(result.Events, now, settings.WorkingStart, settings.WorkingEnd, settings.IncludeAllDay, settings.IncludeTransparent)
		if err != nil {
			state.Err = err.Error()
			return CalendarMsg{ID: id, Result: state}
		}
		state.Available = true
		state.BusyMinutes = calendar.BusyMinutes(intervals)
		state.MeetingCount = len(result.Events)
		for _, event := range result.Events {
			state.Events = append(state.Events, ui.PlannerEvent{Summary: event.Summary, Start: event.Start, End: event.End, AllDay: event.AllDay})
		}
		sort.SliceStable(state.Events, func(i, j int) bool { return state.Events[i].Start.Before(state.Events[j].Start) })
		return CalendarMsg{ID: id, Result: state}
	}
}

func planningConstraints(settings config.Config, calendarState CalendarState, now time.Time) domain.PlanningConstraints {
	capacity := settings.Planning.CapacityFor(now.Weekday())
	return domain.PlanningConstraints{
		CapacityMinutes:   int(capacity / time.Minute),
		BufferMinutes:     int(settings.Planning.Buffer / time.Minute),
		CalendarMinutes:   calendarState.BusyMinutes,
		MeetingCount:      calendarState.MeetingCount,
		CalendarAvailable: calendarState.Available,
		CalendarStale:     calendarState.Stale,
	}
}

func plannerItems(plan domain.DailyPlan) []ui.PlannerItem {
	items := make([]ui.PlannerItem, 0, len(plan.Obligations)+len(plan.Rollover)+len(plan.Candidates))
	appendItem := func(item domain.PlanItem, group ui.PlannerGroup) {
		items = append(items, ui.PlannerItem{
			UUID:        item.Task.UUID,
			Description: item.Task.Description,
			Project:     item.Task.Project,
			Due:         plannerDate(item.Task),
			Priority:    ui.PriorityLabel(item.Task.Priority),
			Estimate:    estimateText(item.Task),
			Group:       group,
			Selected:    item.Selected,
			Fixed:       item.Fixed,
		})
	}
	for _, item := range plan.Obligations {
		appendItem(item, ui.PlannerObligation)
	}
	for _, item := range plan.Rollover {
		appendItem(item, ui.PlannerRollover)
	}
	for _, item := range plan.Candidates {
		appendItem(item, ui.PlannerCandidate)
	}
	return items
}

func (m *Model) recalculatePlannerSummaryFromUI() {
	if m == nil || !m.Planner.Open {
		return
	}
	lookup := make(map[string]domain.Task, len(m.Tasks))
	for _, task := range m.Tasks {
		lookup[task.UUID] = task
	}
	selectedMinutes := 0
	unestimated := 0
	for _, item := range m.Planner.Items {
		if !item.Selected || item.Fixed {
			continue
		}
		task := lookup[item.UUID]
		if task.Estimate == nil {
			unestimated++
		} else {
			selectedMinutes += task.Estimate.Minutes
		}
	}
	m.Planner.Summary.SelectedMinutes = selectedMinutes
	m.Planner.Summary.Unestimated = unestimated
	m.Planner.Summary.AvailableMinutes = m.Planner.Summary.ConfiguredMinutes - m.Planner.Summary.FixedMinutes - m.Planner.Summary.CalendarMinutes - m.Planner.Summary.BufferMinutes
}

func plannerSummary(capacity domain.CapacitySummary, calendarState CalendarState) ui.PlannerSummary {
	return ui.PlannerSummary{
		ConfiguredMinutes: capacity.ConfiguredMinutes,
		FixedMinutes:      capacity.FixedMinutes,
		CalendarMinutes:   capacity.CalendarMinutes,
		BufferMinutes:     capacity.BufferMinutes,
		AvailableMinutes:  capacity.AvailableMinutes,
		SelectedMinutes:   capacity.SelectedMinutes,
		Unestimated:       capacity.UnestimatedSelected,
		UnestimatedFixed:  capacity.UnestimatedFixed,
		MeetingCount:      capacity.MeetingCount,
		CalendarAvailable: calendarState.Available,
		CalendarStale:     calendarState.Stale,
		CalendarError:     calendarState.Err,
		CalendarEvents:    append([]ui.PlannerEvent(nil), calendarState.Events...),
	}
}

func plannerDate(task domain.Task) string {
	if task.Due != nil {
		return task.Due.In(time.Local).Format("Jan 2 15:04")
	}
	if task.Scheduled != nil {
		return "scheduled " + task.Scheduled.In(time.Local).Format("Jan 2 15:04")
	}
	return ""
}

func estimateText(task domain.Task) string {
	if task.Estimate == nil {
		return ""
	}
	return task.Estimate.String()
}

// FormatCalendarState is used by diagnostics/docs-facing tests without
// exposing event content.
func FormatCalendarState(state CalendarState) string {
	if state.Err != "" {
		return "unavailable: " + state.Err
	}
	if !state.Available {
		return "not configured"
	}
	stale := ""
	if state.Stale {
		stale = ", stale"
	}
	return fmt.Sprintf("%d meetings, %d busy minutes%s", state.MeetingCount, state.BusyMinutes, stale)
}
