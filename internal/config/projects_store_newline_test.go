package config

import (
	"context"
	"os"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/internal/domain"
)

func TestProjectCatalogStorePreservesCRLFForNewArrayTables(t *testing.T) {
	path := writeConfig(t, "theme = \"dark\"\r\nrefresh_interval = \"3m\"\r\n")
	store := NewProjectCatalogStore(path)
	snapshot, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), snapshot, domain.ProjectCatalog{{Name: "Work", Value: "work"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range data {
		if b == '\n' && (i == 0 || data[i-1] != '\r') {
			t.Fatalf("found bare LF at byte %d in %q", i, data)
		}
	}
	if _, err := LoadFile(path); err != nil {
		t.Fatal(err)
	}
}
