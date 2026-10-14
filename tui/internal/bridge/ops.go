package bridge

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/quickadd"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/taskwarrior"
)

var versionRE = regexp.MustCompile(`(?i)\b([0-9]+)(?:\.[0-9]+)+\b`)

func (s *Server) status(ctx context.Context) (*statusResult, error) {
	version, err := s.client.Version(ctx)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errors.New("Taskwarrior executable not found; install Taskwarrior 3.x")
		}
		return nil, fmt.Errorf("cannot run Taskwarrior: %w", err)
	}
	match := versionRE.FindStringSubmatch(version)
	if major, convErr := strconv.Atoi(safeIndex(match, 1)); convErr != nil || major < 3 {
		return nil, fmt.Errorf("Taskwarrior 3.x is required; found %s", taskwarrior.Redact(version))
	}
	configured, err := s.client.SyncConfigured(ctx)
	if err != nil {
		configured = false
	}
	uda, _ := s.client.EstimateUDAReadiness(ctx)
	return &statusResult{
		TaskwarriorVersion: version,
		SyncConfigured:     configured,
		EstimateUDA:        string(uda),
		ConfigPath:         s.configPath,
		Sync: syncSettings{
			Enabled:              s.config.Sync.Enabled,
			Startup:              s.config.Sync.Startup,
			Shutdown:             s.config.Sync.Shutdown,
			IntervalSeconds:      int(s.config.Sync.Interval / time.Second),
			MutationDelaySeconds: int(s.config.Sync.MutationDelay / time.Second),
		},
		RefreshIntervalSeconds: int(s.config.RefreshInterval / time.Second),
	}, nil
}

func safeIndex(values []string, index int) string {
	if index < len(values) {
		return values[index]
	}
	return ""
}

// load exports pending and recent completed tasks and derives every view with
// the same domain functions the TUI uses.
func (s *Server) load(ctx context.Context) (*loadResult, error) {
	now := s.now()
	pending, err := s.client.ExportPending(ctx)
	if err != nil {
		return nil, err
	}
	discovered, _ := s.client.Projects(ctx)
	catalog := s.config.Projects.Merge(discovered)
	labels := catalog.Labels()
	s.mu.Lock()
	s.projects = catalog
	s.tags = userTags(pending)
	s.mu.Unlock()

	result := &loadResult{Tasks: []taskWire{}, Today: []section{}, Inbox: []section{}, Completed: []section{}, Projects: []projectWire{}}
	for _, task := range pending {
		result.Tasks = append(result.Tasks, newTaskWire(task, now))
	}
	views := domain.BuildViews(pending, now)
	for _, group := range views.Sections {
		result.Today = append(result.Today, section{Title: string(group.Group), UUIDs: uuids(group.Tasks)})
	}
	result.Inbox = inboxSections(views.Inbox, labels)

	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	completed, err := s.client.ExportCompleted(ctx, day.AddDate(0, 0, -30))
	if err != nil {
		result.CompletedError = err.Error()
	} else {
		recent, sections := domain.BuildCompletedView(completed, now, 30)
		for _, task := range recent {
			result.Tasks = append(result.Tasks, newTaskWire(task, now))
		}
		for _, group := range sections {
			result.Completed = append(result.Completed, section{Title: string(group.Group), UUIDs: uuids(group.Tasks)})
		}
	}

	if name, err := s.client.Context(ctx); err == nil {
		result.Context = name
	}
	for _, project := range catalog {
		result.Projects = append(result.Projects, projectWire{Value: project.Value, Label: labels[project.Value]})
	}
	return result, nil
}

// inboxSections splits the project-sorted Inbox into one section per project.
func inboxSections(tasks []domain.Task, labels map[string]string) []section {
	sections := []section{}
	for _, task := range tasks {
		if n := len(sections); n > 0 && sections[n-1].Project == task.Project {
			sections[n-1].UUIDs = append(sections[n-1].UUIDs, task.UUID)
			continue
		}
		title := "No project"
		if task.Project != "" {
			title = task.Project
			if label := labels[task.Project]; label != "" {
				title = label
			}
		}
		sections = append(sections, section{Title: title, Project: task.Project, UUIDs: []string{task.UUID}})
	}
	return sections
}

func uuids(tasks []domain.Task) []string {
	values := make([]string, len(tasks))
	for i, task := range tasks {
		values[i] = task.UUID
	}
	return values
}

func userTags(tasks []domain.Task) []string {
	seen := map[string]bool{}
	tags := []string{}
	for _, task := range tasks {
		for _, tag := range task.Tags {
			if tag != "" && !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}
	sort.Strings(tags)
	return tags
}

func (s *Server) interpret(p interpretParams) (*interpretationWire, error) {
	value, err := quickadd.InterpretWithLiterals(p.Input, s.now(), quickaddSpans(p.Literals))
	if err != nil {
		return nil, err
	}
	return newInterpretationWire(value, 0), nil
}

func (s *Server) suggest(p suggestParams) suggestResult {
	ctx := quickadd.ContextAt(p.Input, p.Cursor)
	if !ctx.Active {
		return suggestResult{Suggestions: []suggestionWire{}}
	}
	s.mu.Lock()
	projects, tags := s.projects, s.tags
	s.mu.Unlock()
	result := suggestResult{Active: true, Kind: string(ctx.Kind), Start: ctx.TokenStart, End: ctx.TokenEnd, Suggestions: []suggestionWire{}}
	for _, suggestion := range quickadd.SuggestionsWithProjectLabels(ctx, projects.Values(), tags, projects.Labels(), s.now()) {
		input, cursor := quickadd.ApplySuggestion(p.Input, ctx, suggestion)
		result.Suggestions = append(result.Suggestions, suggestionWire{Value: suggestion.Value, Text: suggestion.Text, Label: suggestion.Label, Input: input, Cursor: cursor})
	}
	return result
}

// capture adds the task when the interpretation is valid, matching the TUI's
// one-Enter policy, and otherwise returns it for correction review.
func (s *Server) capture(ctx context.Context, p interpretParams) (*captureResult, error) {
	value, err := quickadd.InterpretWithLiterals(p.Input, s.now(), quickaddSpans(p.Literals))
	if err != nil {
		return nil, err
	}
	if !value.Valid {
		return &captureResult{Review: newInterpretationWire(value, s.storeReview(value))}, nil
	}
	if err := s.client.Add(ctx, value.Task); err != nil {
		return nil, err
	}
	return &captureResult{Added: true}, nil
}

func (s *Server) captureReview(ctx context.Context, p reviewParams) error {
	s.mu.Lock()
	review, ok := s.reviews[p.Token]
	s.mu.Unlock()
	if !ok {
		return errors.New("this review expired; capture the task again")
	}
	task, err := quickadd.FinalizeReview(review, p.Values, p.Touched, p.Decisions)
	if err != nil {
		return err
	}
	if err := s.client.Add(ctx, task); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.reviews, p.Token)
	s.mu.Unlock()
	return nil
}

// captureExplicit adds the task using trigger syntax only, ignoring natural
// language, like the TUI's review escape hatch.
func (s *Server) captureExplicit(ctx context.Context, p interpretParams) error {
	task, err := quickadd.Parse(p.Input)
	if err != nil {
		return err
	}
	return s.client.Add(ctx, task)
}

func (s *Server) storeReview(value quickadd.Interpretation) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextReview++
	s.reviews[s.nextReview] = value
	delete(s.reviews, s.nextReview-maxReviews)
	return s.nextReview
}

func quickaddSpans(values []span) []quickadd.Span {
	spans := make([]quickadd.Span, len(values))
	for i, value := range values {
		spans[i] = quickadd.Span{Start: value.Start, End: value.End}
	}
	return spans
}

func (s *Server) modify(ctx context.Context, p modifyParams) (*modifyResult, error) {
	before := domain.EditSnapshot{
		Description: p.Before.Description,
		Project:     p.Before.Project,
		Priority:    p.Before.Priority,
		Due:         p.Before.Due,
		Scheduled:   p.Before.Scheduled,
		Recurrence:  p.Before.Recurrence,
		Estimate:    estimateValue(p.Before.Estimate),
		Tags:        splitTags(p.Before.Tags),
	}
	after, err := validateEdit(before, p.After)
	if err != nil {
		return nil, err
	}
	diff := domain.Diff(before, after)
	if diff.Empty() {
		return &modifyResult{}, nil
	}
	if err := s.client.Modify(ctx, p.UUID, diff); err != nil {
		return nil, err
	}
	return &modifyResult{Changed: true}, nil
}

// validateEdit applies the TUI editor's submit rules to the edited values.
func validateEdit(before domain.EditSnapshot, fields editFields) (domain.EditSnapshot, error) {
	if strings.TrimSpace(fields.Description) == "" {
		return domain.EditSnapshot{}, fieldError("description", "description cannot be empty")
	}
	priority := normalizePriority(fields.Priority)
	switch priority {
	case "", "H", "M", "L":
	default:
		return domain.EditSnapshot{}, fieldError("priority", "priority must be H, M, L, or empty")
	}
	if err := validateEstimate(before.Estimate, fields.Estimate); err != nil {
		return domain.EditSnapshot{}, fieldError("estimate", err.Error())
	}
	after := domain.EditSnapshot{
		Description: fields.Description,
		Project:     strings.TrimSpace(fields.Project),
		Priority:    priority,
		Due:         strings.TrimSpace(fields.Due),
		Scheduled:   strings.TrimSpace(fields.Scheduled),
		Recurrence:  strings.TrimSpace(fields.Recurrence),
		Estimate:    estimateValue(fields.Estimate),
		Tags:        splitTags(fields.Tags),
	}
	if after.Recurrence != "" {
		recurrence, err := domain.ParseRecurrence(after.Recurrence)
		if err != nil {
			return domain.EditSnapshot{}, fieldError("recurrence", err.Error())
		}
		after.Recurrence = recurrence
		if after.Due == "" {
			return domain.EditSnapshot{}, fieldError("due", "recurring tasks need a first due date")
		}
	}
	return after, nil
}

// validateEstimate accepts a new value within the 24-hour limit, or an
// over-limit value from another client left unchanged.
func validateEstimate(before *domain.Estimate, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	_, err := domain.ParseEstimate(value)
	if err == nil {
		return nil
	}
	if before != nil {
		if parsed, parseErr := domain.ParseEstimateValue(value); parseErr == nil && parsed.Minutes == before.Minutes {
			return nil
		}
	}
	return err
}

func estimateValue(value string) *domain.Estimate {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	estimate, err := domain.ParseEstimateValue(value)
	if err != nil {
		return nil
	}
	return &estimate
}

func normalizePriority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none":
		return ""
	case "h", "high":
		return "H"
	case "m", "medium":
		return "M"
	case "l", "low":
		return "L"
	default:
		return strings.TrimSpace(value)
	}
}

func splitTags(value string) []string {
	seen := map[string]bool{}
	tags := []string{}
	for _, tag := range strings.Fields(value) {
		tag = strings.TrimPrefix(tag, "+")
		if tag != "" && !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

func (s *Server) taskAction(ctx context.Context, op, uuid string) error {
	switch op {
	case "complete":
		return s.client.Complete(ctx, uuid)
	case "start":
		return s.client.Start(ctx, uuid)
	case "stop":
		return s.client.Stop(ctx, uuid)
	case "delete":
		return s.client.Delete(ctx, uuid)
	default:
		return s.client.StopRecurrence(ctx, uuid)
	}
}
