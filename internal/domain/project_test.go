package domain

import (
	"reflect"
	"testing"
)

func TestProjectCatalogMergeDoesNotMutateSources(t *testing.T) {
	catalog := ProjectCatalog{{Name: "Work", Value: "work"}, {Name: "Client", Value: "work.client"}}
	discovered := []string{"work", "personal", "personal", ""}
	got := catalog.Merge(discovered)
	want := []string{"work", "work.client", "personal"}
	if !reflect.DeepEqual(got.Values(), want) {
		t.Fatalf("values=%v want=%v", got.Values(), want)
	}
	if got.Labels()["work.client"] != "Work → Client" {
		t.Fatalf("labels=%v", got.Labels())
	}
	got[0].Name = "Changed"
	if catalog[0].Name != "Work" || discovered[0] != "work" {
		t.Fatal("merge mutated source")
	}
	if len(catalog.Merge(nil)) != 2 {
		t.Fatal("empty discovery erased catalog")
	}
}

func TestProjectLabelsWithMissingAncestors(t *testing.T) {
	catalog := ProjectCatalog{{Name: "Client", Value: "work.client"}}
	if got := catalog.Labels()["work.client"]; got != "work → Client" {
		t.Fatalf("label=%q", got)
	}
}
