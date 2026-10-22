package bridge

import (
	"context"
	"fmt"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

const planDateLayout = "2006-01-02"

// plan derives today's plan with the TUI's rules: obligations are fixed,
// rollover starts selected, and everything else is a candidate.
func (s *Server) plan(ctx context.Context) (*planResult, error) {
	now := s.now()
	pending, err := s.client.ExportPending(ctx)
	if err != nil {
		return nil, err
	}
	plan := domain.BuildDailyPlan(pending, now, s.planningConstraints(now))
	return &planResult{
		Date:            plan.Date.Format(planDateLayout),
		Tag:             domain.DailyPlanTagFor(plan.Date),
		CapacityMinutes: plan.Capacity.ConfiguredMinutes,
		BufferMinutes:   plan.Capacity.BufferMinutes,
		Obligations:     planItems(plan.Obligations, now),
		Rollover:        planItems(plan.Rollover, now),
		Candidates:      planItems(plan.Candidates, now),
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

// summarizePlan sums today's plan for the sidebar. Rollover counts only once
// a saved plan tags it for today.
func (s *Server) summarizePlan(pending []domain.Task, now time.Time) planSummary {
	if !s.config.Planning.Enabled {
		return planSummary{}
	}
	plan := domain.BuildDailyPlan(pending, now, s.planningConstraints(now))
	summary := planSummary{
		Enabled:          true,
		FixedMinutes:     plan.Capacity.FixedMinutes,
		UnestimatedFixed: plan.Capacity.UnestimatedFixed,
		CapacityMinutes:  plan.Capacity.ConfiguredMinutes,
		BufferMinutes:    plan.Capacity.BufferMinutes,
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

func (s *Server) planningConstraints(now time.Time) domain.PlanningConstraints {
	return domain.PlanningConstraints{
		CapacityMinutes: int(s.config.Planning.CapacityFor(now.Weekday()) / time.Minute),
		BufferMinutes:   int(s.config.Planning.Buffer / time.Minute),
	}
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
