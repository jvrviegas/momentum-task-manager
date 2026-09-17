package ui

import (
	"fmt"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/jvrviegas/momentum/internal/domain"
)

// SearchCommitMsg keeps a local filter active after Enter.
type SearchCommitMsg struct{ Query string }

// SearchClearMsg removes the local filter without touching Taskwarrior.
type SearchClearMsg struct{}

// SearchModel owns the local in-memory fuzzy filter.
type SearchModel struct {
	Input  textinput.Model
	Query  string
	Active bool
	Open   bool
	Width  int
	Height int
	Styles Styles
	Icons  Icons
}

func NewSearch(styles Styles, icons Icons) SearchModel {
	input := textinput.New()
	input.Prompt = "/ "
	input.Placeholder = "Filter current view (project:name)"
	return SearchModel{Input: input, Styles: styles, Icons: icons}
}

func (s *SearchModel) SetSize(width, height int) {
	s.Width, s.Height = width, height
	s.Input.SetWidth(max(1, width-4))
}

func (s *SearchModel) OpenSearch(query string) tea.Cmd {
	s.Open = true
	s.Active = query != ""
	s.Query = query
	s.Input.SetValue(query)
	s.Input.CursorEnd()
	return s.Input.Focus()
}

func (s *SearchModel) Clear() {
	s.Open = false
	s.Active = false
	s.Query = ""
	s.Input.Reset()
	s.Input.Blur()
}

func (s *SearchModel) Update(msg tea.Msg) (*SearchModel, tea.Cmd) {
	if !s.Open {
		return s, nil
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "enter":
			s.Query = s.Input.Value()
			s.Active = strings.TrimSpace(s.Query) != ""
			s.Open = false
			s.Input.Blur()
			return s, func() tea.Msg { return SearchCommitMsg{Query: s.Query} }
		case "esc", "escape":
			s.Clear()
			return s, func() tea.Msg { return SearchClearMsg{} }
		}
	}
	var cmd tea.Cmd
	s.Input, cmd = s.Input.Update(msg)
	s.Query = s.Input.Value()
	return s, cmd
}

func (s SearchModel) View() string {
	if !s.Open || s.Width <= 0 || s.Height <= 0 {
		return ""
	}
	return s.Styles.Panel.Width(s.Width).Render(Truncate(s.Input.View(), s.Width))
}

// FilterTasks applies an optional exact project qualifier plus a fuzzy match
// over description, project, and tags without invoking the Taskwarrior adapter.
func FilterTasks(tasks []domain.Task, query string) []domain.Task {
	project, query, hasProject := parseSearchQuery(query)
	if query == "" && !hasProject {
		return append([]domain.Task(nil), tasks...)
	}
	result := make([]domain.Task, 0, len(tasks))
	for _, task := range tasks {
		if _, ok := taskMatchScore(task, project, query, hasProject); ok {
			result = append(result, task)
		}
	}
	return result
}

// MatchesTask reports whether a task satisfies the project qualifier and fuzzy
// search terms in query.
func MatchesTask(task domain.Task, query string) bool {
	project, text, hasProject := parseSearchQuery(query)
	_, ok := taskMatchScore(task, project, text, hasProject)
	return ok
}

func parseSearchQuery(query string) (project, text string, hasProject bool) {
	terms := strings.Fields(strings.ToLower(query))
	textTerms := make([]string, 0, len(terms))
	for _, term := range terms {
		if strings.HasPrefix(term, "project:") && !hasProject {
			project = strings.TrimPrefix(term, "project:")
			hasProject = true
			continue
		}
		textTerms = append(textTerms, term)
	}
	return project, strings.Join(textTerms, " "), hasProject
}

func taskMatchScore(task domain.Task, project, query string, hasProject bool) (int, bool) {
	if hasProject {
		matchesProject := strings.EqualFold(task.Project, project)
		if project == "none" {
			matchesProject = task.Project == ""
		}
		if !matchesProject {
			return 0, false
		}
	}
	if query == "" {
		return 0, true
	}
	values := []string{task.Description, task.Project, strings.Join(task.Tags, " ")}
	best := -1
	for _, value := range values {
		if score, ok := fuzzyMatchScore(query, value); ok && score > best {
			best = score
		}
	}
	return best, best >= 0
}

func fuzzyMatchScore(query, value string) (int, bool) {
	queryRunes := []rune(strings.ToLower(query))
	valueRunes := []rune(strings.ToLower(value))
	if len(queryRunes) == 0 {
		return 0, true
	}
	if strings.HasPrefix(string(valueRunes), string(queryRunes)) {
		return 10000 - len(valueRunes), true
	}
	queryIndex := 0
	score := 0
	last := -2
	for index, valueRune := range valueRunes {
		if queryIndex >= len(queryRunes) || valueRune != queryRunes[queryIndex] {
			continue
		}
		if index == last+1 {
			score += 35
		} else {
			score += 10
		}
		if index == 0 || unicode.IsSpace(valueRunes[index-1]) || strings.ContainsRune("._-", valueRunes[index-1]) {
			score += 20
		}
		last = index
		queryIndex++
	}
	if queryIndex != len(queryRunes) {
		return 0, false
	}
	return score - (len(valueRunes) - len(queryRunes)), true
}

// SearchSummary is the footer value shown while filtering.
func SearchSummary(total, matches int, query string) string {
	if strings.TrimSpace(query) == "" {
		return ""
	}
	return fmt.Sprintf("%d/%d matches", matches, total)
}
