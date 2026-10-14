package domain

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Project is a Momentum suggestion, not a Taskwarrior task or project record.
// Value is the complete dotted Taskwarrior path; Name labels its final segment.
type Project struct {
	Name  string `toml:"name"`
	Value string `toml:"value"`
}

// ProjectCatalog preserves the user's configured suggestion order.
type ProjectCatalog []Project

var projectValuePattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

func (c ProjectCatalog) Validate() error {
	seen := make(map[string]bool, len(c))
	for i, project := range c {
		if strings.TrimSpace(project.Name) == "" || strings.IndexFunc(project.Name, unicode.IsControl) >= 0 {
			return fmt.Errorf("config key projects[%d].name must be a non-empty display name without control characters", i)
		}
		if !projectValuePattern.MatchString(project.Value) {
			return fmt.Errorf("config key projects[%d].value must use lowercase letters/numbers, hyphens, and dots between hierarchy segments", i)
		}
		if seen[project.Value] {
			return fmt.Errorf("config key projects[%d].value duplicates %q", i, project.Value)
		}
		seen[project.Value] = true
	}
	return nil
}

// Merge keeps configured projects first and adds unique discovered values.
// It never changes either source or any task assignments.
func (c ProjectCatalog) Merge(discovered []string) ProjectCatalog {
	result := append(ProjectCatalog(nil), c...)
	seen := make(map[string]bool, len(c)+len(discovered))
	for _, project := range c {
		seen[project.Value] = true
	}
	for _, value := range discovered {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, Project{Value: value})
	}
	return result
}

func (c ProjectCatalog) Values() []string {
	values := make([]string, len(c))
	for i, project := range c {
		values[i] = project.Value
	}
	return values
}

// Labels resolves dotted ancestors into readable breadcrumbs. Discovered
// projects without display names retain their original Taskwarrior values.
func (c ProjectCatalog) Labels() map[string]string {
	names := make(map[string]string, len(c))
	for _, project := range c {
		names[project.Value] = strings.TrimSpace(project.Name)
	}
	labels := make(map[string]string, len(c))
	for _, project := range c {
		if project.Name == "" {
			continue
		}
		parts := strings.Split(project.Value, ".")
		labelsForPath := make([]string, len(parts))
		for i, part := range parts {
			label := names[strings.Join(parts[:i+1], ".")]
			if label == "" {
				label = part
			}
			labelsForPath[i] = label
		}
		labels[project.Value] = strings.Join(labelsForPath, " → ")
	}
	return labels
}
