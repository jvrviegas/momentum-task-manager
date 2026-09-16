package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestComposeProjectValueAndDraftResolution(t *testing.T) {
	tests := []struct {
		name    string
		parent  string
		segment string
		want    string
		wantErr bool
	}{
		{name: "root", segment: "work", want: "work"},
		{name: "child", parent: "work", segment: "client", want: "work.client"},
		{name: "missing configured parent is allowed", parent: "missing", segment: "child", want: "missing.child"},
		{name: "invalid parent", parent: "Work", segment: "client", wantErr: true},
		{name: "invalid local segment", parent: "work", segment: "client.deep", wantErr: true},
		{name: "empty local segment", parent: "work", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ComposeProjectValue(tc.parent, tc.segment)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("value=%q err=%v want=%q", got, err, tc.want)
			}
		})
	}

	draft := ProjectDraft{Name: "Client", ParentValue: "work", ValueSegment: "client"}
	project, err := draft.Project()
	if err != nil {
		t.Fatal(err)
	}
	if project != (Project{Name: "Client", Value: "work.client"}) {
		t.Fatalf("project=%#v", project)
	}
	if got := DraftForProject(Project{Name: "Client", Value: "work.client"}); got != draft {
		t.Fatalf("draft=%#v want=%#v", got, draft)
	}
}

func TestPlanAddProjectCopiesCatalogAndRejectsDuplicates(t *testing.T) {
	catalog := ProjectCatalog{{Name: "Work", Value: "work"}}
	plan, err := PlanAddProject(catalog, Project{Name: "Personal", Value: "personal"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != ProjectEditAdd || !reflect.DeepEqual(plan.Before.Values(), []string{"work"}) || !reflect.DeepEqual(plan.After.Values(), []string{"work", "personal"}) {
		t.Fatalf("plan=%#v", plan)
	}

	plan.After[0].Name = "Changed"
	if catalog[0].Name != "Work" || plan.Before[0].Name != "Work" {
		t.Fatal("plan or input shares catalog storage")
	}

	if _, err := PlanAddProject(catalog, Project{Name: "Other", Value: "work"}); err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestPlanUpdateProjectClassifiesChangesAndMapsConfiguredDescendants(t *testing.T) {
	catalog := ProjectCatalog{
		{Name: "Work", Value: "work"},
		{Name: "Client", Value: "work.client"},
		{Name: "Deep", Value: "work.client.deep"},
		{Name: "Workshop", Value: "workshop"},
	}

	tests := []struct {
		name            string
		destination     Project
		includeSub      bool
		wantKind        ProjectEditKind
		wantValues      []string
		wantMappings    []ProjectValueMapping
		wantSource      string
		wantDestination string
	}{
		{
			name:            "no-op",
			destination:     Project{Name: "Work", Value: "work"},
			wantKind:        ProjectEditNoOp,
			wantValues:      []string{"work", "work.client", "work.client.deep", "workshop"},
			wantSource:      "work",
			wantDestination: "work",
		},
		{
			name:            "label-only",
			destination:     Project{Name: "Delivery", Value: "work"},
			includeSub:      true,
			wantKind:        ProjectEditLabelOnly,
			wantValues:      []string{"work", "work.client", "work.client.deep", "workshop"},
			wantSource:      "work",
			wantDestination: "work",
		},
		{
			name:         "exact-value-only",
			destination:  Project{Name: "Delivery", Value: "delivery"},
			wantKind:     ProjectEditValue,
			wantValues:   []string{"delivery", "work.client", "work.client.deep", "workshop"},
			wantMappings: []ProjectValueMapping{{OldValue: "work", NewValue: "delivery"}},
			wantSource:   "work", wantDestination: "delivery",
		},
		{
			name:        "configured-subtree",
			destination: Project{Name: "Delivery", Value: "delivery"},
			includeSub:  true,
			wantKind:    ProjectEditValue,
			wantValues:  []string{"delivery", "delivery.client", "delivery.client.deep", "workshop"},
			wantMappings: []ProjectValueMapping{
				{OldValue: "work", NewValue: "delivery"},
				{OldValue: "work.client", NewValue: "delivery.client"},
				{OldValue: "work.client.deep", NewValue: "delivery.client.deep"},
			},
			wantSource: "work", wantDestination: "delivery",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := PlanUpdateProject(catalog, "work", tc.destination, tc.includeSub)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Kind != tc.wantKind || !reflect.DeepEqual(plan.After.Values(), tc.wantValues) || !reflect.DeepEqual(plan.Mappings, tc.wantMappings) {
				t.Fatalf("kind=%q values=%v mappings=%v", plan.Kind, plan.After.Values(), plan.Mappings)
			}
			if plan.SourceValue != tc.wantSource || plan.DestinationValue != tc.wantDestination {
				t.Fatalf("source=%q destination=%q", plan.SourceValue, plan.DestinationValue)
			}
		})
	}

	catalog[0].Name = "Still Work"
	catalog[1].Value = "work.client"
	plan, err := PlanUpdateProject(catalog, "work", Project{Name: "Delivery", Value: "delivery"}, true)
	if err != nil {
		t.Fatal(err)
	}
	plan.After[1].Name = "Changed"
	if catalog[0].Name != "Still Work" || catalog[1].Name != "Client" {
		t.Fatal("update plan mutated its input")
	}
}

func TestPlanUpdateAllowsMissingConfiguredAncestorsAndPreservesSuffixes(t *testing.T) {
	catalog := ProjectCatalog{
		{Name: "Leaf Parent", Value: "old.branch"},
		{Name: "Leaf", Value: "old.branch.leaf"},
	}
	plan, err := PlanUpdateProject(catalog, "old.branch", Project{Name: "New Branch", Value: "new.branch"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.After.Values(), []string{"new.branch", "new.branch.leaf"}) {
		t.Fatalf("values=%v", plan.After.Values())
	}
	want := []ProjectValueMapping{
		{OldValue: "old.branch", NewValue: "new.branch"},
		{OldValue: "old.branch.leaf", NewValue: "new.branch.leaf"},
	}
	if !reflect.DeepEqual(plan.Mappings, want) {
		t.Fatalf("mappings=%v want=%v", plan.Mappings, want)
	}
}

func TestPlanUpdateRejectsOwnDescendantAndResultingCollisions(t *testing.T) {
	tests := []struct {
		name    string
		catalog ProjectCatalog
		source  string
		dest    string
		include bool
	}{
		{
			name:    "move parent under own descendant",
			catalog: ProjectCatalog{{Name: "Work", Value: "work"}, {Name: "Client", Value: "work.client"}},
			source:  "work", dest: "work.client",
		},
		{
			name:    "unchanged destination collision",
			catalog: ProjectCatalog{{Name: "Work", Value: "work"}, {Name: "Personal", Value: "personal"}},
			source:  "work", dest: "personal",
		},
		{
			name: "mapped descendant collision",
			catalog: ProjectCatalog{
				{Name: "Old", Value: "old"},
				{Name: "Child", Value: "old.child"},
				{Name: "New", Value: "new"},
				{Name: "Existing Child", Value: "new.child"},
			},
			source: "old", dest: "new", include: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := PlanUpdateProject(tc.catalog, tc.source, Project{Name: "Destination", Value: tc.dest}, tc.include)
			if err == nil {
				t.Fatal("expected update to be rejected")
			}
		})
	}

	catalog := ProjectCatalog{{Name: "Work", Value: "work"}, {Name: "Workshop", Value: "workshop"}, {Name: "Child", Value: "work.child"}}
	plan, err := PlanUpdateProject(catalog, "work", Project{Name: "New", Value: "new"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.After.Values(), []string{"new", "workshop", "new.child"}) {
		t.Fatalf("prefix lookalike was changed: %v", plan.After.Values())
	}
}

func TestPlanUpdateRequiresExactSourceOwnership(t *testing.T) {
	catalog := ProjectCatalog{{Name: "Work", Value: "work"}, {Name: "Workshop", Value: "workshop"}}
	if _, err := PlanUpdateProject(catalog, "wor", Project{Name: "New", Value: "new"}, false); err == nil {
		t.Fatal("prefix source should not select an entry")
	}
	if _, err := PlanRemoveProject(catalog, "wor", false); err == nil {
		t.Fatal("prefix source should not remove an entry")
	}
}

func TestPlanRemoveProjectHonorsSubtreeChoiceWithoutMutatingInput(t *testing.T) {
	catalog := ProjectCatalog{
		{Name: "Work", Value: "work"},
		{Name: "Client", Value: "work.client"},
		{Name: "Deep", Value: "work.client.deep"},
		{Name: "Workshop", Value: "workshop"},
		{Name: "Personal", Value: "personal"},
	}

	parentOnly, err := PlanRemoveProject(catalog, "work", false)
	if err != nil {
		t.Fatal(err)
	}
	if parentOnly.Kind != ProjectEditRemove || !reflect.DeepEqual(parentOnly.After.Values(), []string{"work.client", "work.client.deep", "workshop", "personal"}) {
		t.Fatalf("parent-only values=%v", parentOnly.After.Values())
	}

	subtree, err := PlanRemoveProject(catalog, "work", true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(subtree.After.Values(), []string{"workshop", "personal"}) {
		t.Fatalf("subtree values=%v", subtree.After.Values())
	}
	if !reflect.DeepEqual(catalog.Values(), []string{"work", "work.client", "work.client.deep", "workshop", "personal"}) {
		t.Fatal("remove plan mutated input")
	}
}
