package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestProjectCatalogLoadsHierarchy(t *testing.T) {
	path := writeConfig(t, `[[projects]]
name = "Carbon Products"
value = "carbon-products"
[[projects]]
name = "Dash2Zero"
value = "carbon-products.dash2zero"
`)
	got, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Projects) != 2 || got.Projects[1].Name != "Dash2Zero" {
		t.Fatalf("projects=%#v", got.Projects)
	}
	if want := []string{"carbon-products", "carbon-products.dash2zero"}; !reflect.DeepEqual(got.Projects.Values(), want) {
		t.Fatalf("values=%v", got.Projects.Values())
	}
	if got.Projects.Labels()["carbon-products.dash2zero"] != "Carbon Products → Dash2Zero" {
		t.Fatalf("labels=%v", got.Projects.Labels())
	}
}

func TestInvalidProjectCatalogRejected(t *testing.T) {
	for _, entry := range []string{
		`name = ""; value = "work"`,
		`name = "Work"; value = ""`,
		`name = "Work"; value = "Work"`,
		`name = "Work"; value = "work space"`,
		`name = "Work"; value = "work..client"`,
		`name = "Work"; value = "work.#client"`,
		`name = "Work"; value = "work"; typo = true`,
		`name = "Work"; value = "work"; [[projects]]; name = "Other"; value = "work"`,
	} {
		t.Run(entry, func(t *testing.T) {
			path := writeConfig(t, "[[projects]]\n"+strings.ReplaceAll(entry, "; ", "\n"))
			_, err := LoadFile(path)
			if err == nil || !strings.Contains(err.Error(), "projects") {
				t.Fatalf("expected projects error, got %v", err)
			}
		})
	}
}

func TestEmptyProjectCatalogIsOptional(t *testing.T) {
	got, err := LoadFile(writeConfig(t, `projects = []`))
	if err != nil || len(got.Projects) != 0 {
		t.Fatalf("config=%#v err=%v", got, err)
	}
	if len(Defaults().Projects) != 0 {
		t.Fatal("personal projects must not become application defaults")
	}
}
