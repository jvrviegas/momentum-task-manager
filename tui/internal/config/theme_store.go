package config

import (
	"context"

	tomledit "github.com/smm-h/go-toml-edit"
)

// ThemeStore persists the top-level theme key in the active config file.
type ThemeStore interface {
	SaveTheme(context.Context, ProjectCatalogSnapshot, string) (ProjectCatalogSnapshot, error)
}

// SaveTheme sets theme with the same conflict checks and atomic replacement
// as Save, so the returned snapshot stays valid for later catalog saves.
func (s *FileProjectCatalogStore) SaveTheme(ctx context.Context, snapshot ProjectCatalogSnapshot, theme string) (ProjectCatalogSnapshot, error) {
	if err := contextError(ctx); err != nil {
		return ProjectCatalogSnapshot{}, err
	}
	return s.save(ctx, snapshot, "theme", func(doc *tomledit.Document) error {
		return doc.Set("theme", theme)
	})
}
