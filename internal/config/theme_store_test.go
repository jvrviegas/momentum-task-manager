package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func TestThemeStoreAddsRootKeyWithoutTouchingTablesOrComments(t *testing.T) {
	path := writeConfig(t, `# keep top comment
refresh_interval = "10m" # keep refresh

[sync]
enabled = false

[[projects]]
name = "Work"
value = "work"
`)
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.SaveTheme(context.Background(), snapshot, ThemeTerminal)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, retained := range []string{"# keep top comment", `refresh_interval = "10m" # keep refresh`, "[sync]", "enabled = false"} {
		if !strings.Contains(text, retained) {
			t.Errorf("lost %q:\n%s", retained, text)
		}
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != ThemeTerminal || got.Sync.Enabled || !reflect.DeepEqual(got.Projects, domain.ProjectCatalog{{Name: "Work", Value: "work"}}) {
		t.Fatalf("theme was not written at the root: %#v\n%s", got, text)
	}
	// The returned snapshot must stay usable for a later catalog save.
	if _, err := store.Save(context.Background(), saved, domain.ProjectCatalog{{Name: "Home", Value: "home"}}); err != nil {
		t.Fatalf("catalog save after theme save: %v", err)
	}
}

func TestThemeStoreReplacesExistingValueAndKeepsItsComment(t *testing.T) {
	path := writeConfig(t, `theme = "auto" # keep theme comment
`)
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveTheme(context.Background(), snapshot, ThemeLight); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if text := string(data); !strings.Contains(text, "# keep theme comment") || strings.Count(text, "theme =") != 1 {
		t.Fatalf("config=%q", text)
	}
	if got, err := LoadFile(path); err != nil || got.Theme != ThemeLight {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func TestThemeStoreCreatesMissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "momentum", "config.toml")
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveTheme(context.Background(), snapshot, ThemeDark); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadFile(path); err != nil || got.Theme != ThemeDark {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func TestThemeStoreRejectsInvalidThemeAndExternalEditsWithoutWriting(t *testing.T) {
	original := "theme = \"auto\"\n"
	path := writeConfig(t, original)
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveTheme(context.Background(), snapshot, "blue"); err == nil || !strings.Contains(err.Error(), "theme") {
		t.Fatalf("expected theme validation error, got %v", err)
	}
	external := "theme = \"dark\"\n"
	if err := os.WriteFile(path, []byte(external), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveTheme(context.Background(), snapshot, ThemeLight); !errors.Is(err, ErrProjectConfigConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != external {
		t.Fatalf("config was overwritten: %q", data)
	}
}
