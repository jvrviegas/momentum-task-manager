package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum/internal/domain"
)

func TestProjectCatalogStorePreservesUnrelatedTOMLAndArrayTableComments(t *testing.T) {
	path := writeConfig(t, `# keep top comment
refresh_interval = "10m" # keep refresh

[[projects]]
# keep first project comment
name = 'Work' # keep first name comment
value = "work"

[sync]
enabled = false
interval = "7m"
mutation_delay = "2s"
startup = false
shutdown = false

# keep between project entries
[[projects]]
name = "Client"
value = "work.client"
`)
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}

	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantBefore := domain.ProjectCatalog{{Name: "Work", Value: "work"}, {Name: "Client", Value: "work.client"}}
	if !reflect.DeepEqual(snapshot.Projects, wantBefore) {
		t.Fatalf("before projects=%#v", snapshot.Projects)
	}

	want := domain.ProjectCatalog{{Name: "Delivery", Value: "delivery"}, {Name: "Customer", Value: "delivery.client"}}
	if _, err := store.Save(context.Background(), snapshot, want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, retained := range []string{
		"# keep top comment",
		"# keep refresh",
		"# keep first project comment",
		"# keep first name comment",
		"# keep between project entries",
		"refresh_interval = \"10m\"",
		"[sync]",
		"mutation_delay = \"2s\"",
	} {
		if !strings.Contains(text, retained) {
			t.Fatalf("saved config lost %q:\n%s", retained, text)
		}
	}
	if strings.Contains(text, "MOMENTUM_ICONS") {
		t.Fatal("environment-derived settings were serialized")
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Projects, want) {
		t.Fatalf("saved projects=%#v want=%#v", loaded.Projects, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%o want 640", info.Mode().Perm())
	}
}

func TestProjectCatalogStoreHandlesRegularArraysAndEmptyCatalogs(t *testing.T) {
	path := writeConfig(t, `# catalog comment
projects = [{name = 'Work', value = "work"}] # keep array comment
refresh_interval = "3m"
`)
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := domain.ProjectCatalog{{Name: "Personal", Value: "personal"}}
	if _, err := store.Save(context.Background(), snapshot, want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "# catalog comment") || !strings.Contains(text, "# keep array comment") || !strings.Contains(text, "refresh_interval = \"3m\"") {
		t.Fatalf("regular array save lost unrelated content:\n%s", text)
	}
	loaded, err := LoadFile(path)
	if err != nil || !reflect.DeepEqual(loaded.Projects, want) {
		t.Fatalf("projects=%#v err=%v", loaded.Projects, err)
	}

	snapshot, err = store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), snapshot, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadFile(path)
	if err != nil || len(loaded.Projects) != 0 {
		t.Fatalf("cleared projects=%#v err=%v", loaded.Projects, err)
	}
}

func TestProjectCatalogStoreCreatesMissingConfigDeliberately(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "config.toml")
	store := NewProjectCatalogStore(path)
	before, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if before.Revision.Exists || before.Projects != nil {
		t.Fatalf("missing snapshot=%#v", before)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Read created config directory: %v", err)
	}

	want := domain.ProjectCatalog{{Name: "Work", Value: "work"}}
	if _, err := store.Save(context.Background(), before, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("new file mode=%o want 600", info.Mode().Perm())
	}
	loaded, err := LoadFile(path)
	if err != nil || !reflect.DeepEqual(loaded.Projects, want) {
		t.Fatalf("projects=%#v err=%v", loaded.Projects, err)
	}
}

func TestProjectCatalogStoreAddsAndClearsArrayTablesWithoutTouchingOtherSettings(t *testing.T) {
	path := writeConfig(t, `theme = "dark"
[sync]
enabled = true
interval = "5m"
mutation_delay = "15s"
startup = true
shutdown = true
`)
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := domain.ProjectCatalog{{Name: "Work", Value: "work"}, {Name: "Client", Value: "work.client"}}
	if _, err := store.Save(context.Background(), snapshot, want); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile(path)
	if err != nil || !reflect.DeepEqual(loaded.Projects, want) {
		t.Fatalf("added projects=%#v err=%v", loaded.Projects, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `theme = "dark"`) {
		t.Fatalf("other settings changed: %s err=%v", data, err)
	}

	snapshot, err = store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), snapshot, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadFile(path)
	if err != nil || len(loaded.Projects) != 0 {
		t.Fatalf("cleared projects=%#v err=%v", loaded.Projects, err)
	}
	data, err = os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "[[projects]]") {
		t.Fatalf("project tables remain after clear: %s err=%v", data, err)
	}
}

func TestProjectCatalogStoreRejectsExternalEditsAndInvalidSourceWithoutOverwrite(t *testing.T) {
	path := writeConfig(t, `theme = "dark"
[[projects]]
name = "Work"
value = "work"
`)
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	external := []byte("theme = \"light\"\n")
	if err := os.WriteFile(path, external, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), snapshot, domain.ProjectCatalog{{Name: "New", Value: "new"}}); !errors.Is(err, ErrProjectConfigConflict) {
		t.Fatalf("external edit error=%v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(got, external) {
		t.Fatalf("external bytes changed=%q err=%v", got, err)
	}

	invalidPath := writeConfig(t, "projects = [\n")
	invalidStore := NewProjectCatalogStore(invalidPath)
	if _, err := invalidStore.Read(context.Background()); err == nil {
		t.Fatal("invalid source was accepted")
	}
	invalidBytes, err := os.ReadFile(invalidPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invalidStore.Save(context.Background(), ProjectCatalogSnapshot{}, domain.ProjectCatalog{{Name: "New", Value: "new"}}); err == nil {
		t.Fatal("invalid source was overwritten")
	}
	got, err = os.ReadFile(invalidPath)
	if err != nil || !reflect.DeepEqual(got, invalidBytes) {
		t.Fatalf("invalid source bytes changed=%q err=%v", got, err)
	}
}

func TestProjectCatalogStoreRejectsReadOnlyTarget(t *testing.T) {
	path := writeConfig(t, `[[projects]]
name = "Work"
value = "work"
`)
	store := NewProjectCatalogStore(path)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), snapshot, domain.ProjectCatalog{{Name: "New", Value: "new"}}); !errors.Is(err, ErrProjectConfigReadOnly) {
		t.Fatalf("read-only error=%v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(got, original) {
		t.Fatalf("read-only bytes changed=%q err=%v", got, err)
	}
}

func TestProjectCatalogStorePreservesSymlinkAndUpdatesResolvedTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions are not consistent on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target.toml")
	link := filepath.Join(root, "config.toml")
	if err := os.WriteFile(target, []byte(`[[projects]]
name = "Work"
value = "work"
`), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(target), link); err != nil {
		t.Fatal(err)
	}
	store := NewProjectCatalogStore(link)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Revision.Symlink || snapshot.Revision.LinkTarget != filepath.Base(target) {
		t.Fatalf("revision=%#v", snapshot.Revision)
	}
	want := domain.ProjectCatalog{{Name: "Delivery", Value: "delivery"}}
	if _, err := store.Save(context.Background(), snapshot, want); err != nil {
		t.Fatal(err)
	}
	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal("save replaced the symlink")
	}
	if got, err := os.Readlink(link); err != nil || got != filepath.Base(target) {
		t.Fatalf("link target=%q err=%v", got, err)
	}
	loaded, err := LoadFile(link)
	if err != nil || !reflect.DeepEqual(loaded.Projects, want) {
		t.Fatalf("target projects=%#v err=%v", loaded.Projects, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("target mode=%o want 640", info.Mode().Perm())
	}
}

func TestProjectCatalogStoreRejectsChangedSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not consistent on Windows")
	}
	root := t.TempDir()
	first := filepath.Join(root, "first.toml")
	second := filepath.Join(root, "second.toml")
	link := filepath.Join(root, "config.toml")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("theme = \"dark\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Base(first), link); err != nil {
		t.Fatal(err)
	}
	store := NewProjectCatalogStore(link)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(second), link); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), snapshot, domain.ProjectCatalog{{Name: "New", Value: "new"}}); !errors.Is(err, ErrProjectConfigConflict) {
		t.Fatalf("changed-link error=%v", err)
	}
	got, err := os.ReadFile(second)
	if err != nil || !reflect.DeepEqual(got, before) {
		t.Fatalf("new symlink target changed=%q err=%v", got, err)
	}
	if target, err := os.Readlink(link); err != nil || target != filepath.Base(second) {
		t.Fatalf("link=%q err=%v", target, err)
	}
}

func TestProjectCatalogStoreValidatesBeforeCreatingMissingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config.toml")
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	invalid := domain.ProjectCatalog{{Name: "One", Value: "same"}, {Name: "Two", Value: "same"}}
	if _, err := store.Save(context.Background(), snapshot, invalid); err == nil {
		t.Fatal("invalid catalog was accepted")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid save created path: %v", err)
	}
}
