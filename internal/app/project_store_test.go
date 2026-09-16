package app

import (
	"context"
	"testing"

	"github.com/jvrviegas/momentum/internal/config"
	"github.com/jvrviegas/momentum/internal/domain"
)

type recordingProjectStore struct {
	readCalls int
	saveCalls int
}

func (s *recordingProjectStore) Read(context.Context) (config.ProjectCatalogSnapshot, error) {
	s.readCalls++
	return config.ProjectCatalogSnapshot{}, nil
}

func (s *recordingProjectStore) Save(context.Context, config.ProjectCatalogSnapshot, domain.ProjectCatalog) (config.ProjectCatalogSnapshot, error) {
	s.saveCalls++
	return config.ProjectCatalogSnapshot{}, nil
}

func TestNewModelInjectsProjectStoreWithoutConstructionTimeIO(t *testing.T) {
	store := &recordingProjectStore{}
	model := NewModel(ModelOptions{
		Client:            &fakeClient{},
		Config:            config.Defaults(),
		InitialView:       ViewInbox,
		ProjectStore:      store,
		ProjectConfigPath: "/tmp/momentum-test-config.toml",
	})
	if model.ProjectStore != store || model.ProjectConfigPath != "/tmp/momentum-test-config.toml" {
		t.Fatalf("store=%#v path=%q", model.ProjectStore, model.ProjectConfigPath)
	}
	if store.readCalls != 0 || store.saveCalls != 0 {
		t.Fatalf("constructor performed store I/O: reads=%d saves=%d", store.readCalls, store.saveCalls)
	}
}

func TestNewModelWithoutProjectStoreDoesNotCreatePersistenceDependency(t *testing.T) {
	model := NewModel(ModelOptions{Config: config.Defaults(), InitialView: ViewInbox})
	if model.ProjectStore != nil || model.ProjectConfigPath != "" {
		t.Fatalf("rendering-only model acquired persistence: store=%#v path=%q", model.ProjectStore, model.ProjectConfigPath)
	}
}
