package bridge

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/calendar"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

const planDateLayout = "2006-01-02"

// plan derives today's plan with the TUI's rules: obligations are fixed,
// rollover starts selected, and everything else is a candidate.
func (s *Server) plan(ctx context.Context, input *calendarInput) (*planResult, error) {
	now := s.now()
	pending, err := s.client.ExportPending(ctx)
	if err != nil {
		return nil, err
	}
	busy := s.readCalendar(ctx, now, input, true)
	plan := domain.BuildDailyPlan(pending, now, s.planningConstraints(now, busy))
	return &planResult{
		Date:            plan.Date.Format(planDateLayout),
		Tag:             domain.DailyPlanTagFor(plan.Date),
		CapacityMinutes: plan.Capacity.ConfiguredMinutes,
		BufferMinutes:   plan.Capacity.BufferMinutes,
		Obligations:     planItems(plan.Obligations, now),
		Rollover:        planItems(plan.Rollover, now),
		Candidates:      planItems(plan.Candidates, now),
		Calendar:        busy,
	}, nil
}

func planItems(items []domain.PlanItem, now time.Time) []planItemWire {
	wires := make([]planItemWire, len(items))
	for i, item := range items {
		wires[i] = planItemWire{Task: newTaskWire(item.Task, now), Selected: item.Selected}
		if item.Group == domain.PlanRollover {
			if planned, ok := domain.PlannedDate(item.Task); ok {
				wires[i].PlannedDate = planned.Format(planDateLayout)
			}
		}
	}
	return wires
}

// summarizePlan sums today's plan for the sidebar, which shows only the
// calendar's busy minutes. Rollover counts only once a saved plan tags it for
// today.
func (s *Server) summarizePlan(ctx context.Context, pending []domain.Task, now time.Time, input *calendarInput) planSummary {
	if !s.config.Planning.Enabled {
		return planSummary{}
	}
	busy := s.readCalendar(ctx, now, input, false)
	plan := domain.BuildDailyPlan(pending, now, s.planningConstraints(now, busy))
	summary := planSummary{
		Enabled:           true,
		FixedMinutes:      plan.Capacity.FixedMinutes,
		UnestimatedFixed:  plan.Capacity.UnestimatedFixed,
		CapacityMinutes:   plan.Capacity.ConfiguredMinutes,
		BufferMinutes:     plan.Capacity.BufferMinutes,
		CalendarMinutes:   plan.Capacity.CalendarMinutes,
		CalendarAvailable: busy.Available,
		CalendarError:     busy.Error,
	}
	for _, item := range plan.Candidates {
		if !item.Selected {
			continue
		}
		summary.ChosenCount++
		if item.Task.Estimate == nil {
			summary.UnestimatedChosen++
		} else {
			summary.ChosenMinutes += item.Task.Estimate.Minutes
		}
	}
	return summary
}

func (s *Server) planningConstraints(now time.Time, busy calendarWire) domain.PlanningConstraints {
	return domain.PlanningConstraints{
		CapacityMinutes:   int(s.config.Planning.CapacityFor(now.Weekday()) / time.Minute),
		BufferMinutes:     int(s.config.Planning.Buffer / time.Minute),
		CalendarMinutes:   busy.BusyMinutes,
		MeetingCount:      busy.EventCount,
		CalendarAvailable: busy.Available,
		CalendarStale:     busy.Stale,
	}
}

// readCalendar reads today's busy time from the events the app handed over
// when its source is the Mac's Calendar, which count over everything else
// (ADR 0003, D9); from Google Calendar, through the server, when [calendar]
// source is "google"; and otherwise from the [calendar] files as the TUI's
// planner does. Google counts whatever [calendar] enabled says. A file that
// cannot be read or parsed, calendars the app could not read, or a Google
// Calendar the server could not read make the whole calendar unavailable, and
// planning goes on with tasks only. fresh is Plan Today's: it asks Google
// again instead of reusing what a load kept.
func (s *Server) readCalendar(ctx context.Context, now time.Time, input *calendarInput, fresh bool) calendarWire {
	settings := s.config.Calendar
	google := input == nil && settings.Source == config.CalendarSourceGoogle
	busy := calendarWire{Enabled: settings.Enabled || input != nil || google, WorkingStart: settings.WorkingStart, WorkingEnd: settings.WorkingEnd, Blocks: []busyBlock{}}
	if !busy.Enabled {
		return busy
	}
	options := calendar.Options{
		IncludeAllDay:      settings.IncludeAllDay,
		IncludeTransparent: settings.IncludeTransparent,
		WorkingStart:       settings.WorkingStart,
		WorkingEnd:         settings.WorkingEnd,
		StaleAfter:         settings.StaleAfter,
		Now:                now,
	}
	var loaded calendar.LoadResult
	switch {
	case input != nil:
		if input.Error != "" {
			busy.Error = input.Error
			return busy
		}
		loaded = calendar.FromEvents(input.events(), options)
	case google:
		events, err := s.googleToday(ctx, now, fresh)
		if err != nil {
			busy.Error = err.Error()
			return busy
		}
		loaded = calendar.FromEvents(events, options)
	default:
		home, _ := os.UserHomeDir()
		options.Paths = settings.PathsFor(home)
		loaded = calendar.Load(ctx, options)
	}
	busy.Stale = loaded.Stale
	if loaded.Err != nil {
		busy.Error = loaded.Err.Error()
		return busy
	}
	intervals, err := calendar.BusyIntervalsWithOptions(loaded.Events, now, settings.WorkingStart, settings.WorkingEnd, settings.IncludeAllDay, settings.IncludeTransparent)
	if err != nil {
		busy.Error = err.Error()
		return busy
	}
	busy.Available = true
	busy.BusyMinutes = calendar.BusyMinutes(intervals)
	busy.EventCount = len(loaded.Events)
	events := append([]calendar.Event(nil), loaded.Events...)
	sort.SliceStable(events, func(i, j int) bool { return events[i].Start.Before(events[j].Start) })
	for _, interval := range intervals {
		block := busyBlock{Start: stamp(&interval.Start), End: stamp(&interval.End), Titles: []string{}}
		for _, event := range events {
			if event.Summary != "" && event.Start.Before(interval.End) && event.End.After(interval.Start) {
				block.Titles = append(block.Titles, event.Summary)
			}
		}
		busy.Blocks = append(busy.Blocks, block)
	}
	return busy
}

// applyPlan tags the selected tasks for the given day and untags the rest,
// one task at a time. It builds the changes from a fresh export, so a task
// completed since the sheet opened is skipped and a retry after a failure
// applies only what is left.
func (s *Server) applyPlan(ctx context.Context, p applyPlanParams) (*applyPlanResult, error) {
	date, err := time.ParseInLocation(planDateLayout, p.Date, s.now().Location())
	if err != nil {
		return nil, fmt.Errorf("invalid plan date %q", p.Date)
	}
	pending, err := s.client.ExportPending(ctx)
	if err != nil {
		return nil, err
	}
	selected := make(map[string]bool, len(p.UUIDs))
	for _, uuid := range p.UUIDs {
		selected[uuid] = true
	}
	mutations := domain.BuildPlanMutations(pending, selected, date)
	result := &applyPlanResult{Total: len(mutations)}
	for _, mutation := range mutations {
		if err := s.client.Modify(ctx, mutation.UUID, mutation.Diff); err != nil {
			result.Error = err.Error()
			return result, nil
		}
		result.Applied++
	}
	return result, nil
}
