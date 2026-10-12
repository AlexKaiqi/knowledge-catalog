package home

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"testing"
)

// A-01: provider selection is one configuration axis. Application services
// are assembled after openMembers and therefore cannot vary with this field.
func TestAuthoritySelectionChangesOnlyRepositoryProfileField(t *testing.T) {
	base := DefaultStores()
	base.Index = "none"
	candidate := base
	candidate.Repository = "lakefs"
	candidate = candidate.withDefaults()
	if err := candidate.ValidateProfile(); err != nil {
		t.Fatal(err)
	}
	driver, err := authorityFor(candidate.Repository)
	if err != nil || driver.open == nil || driver.discover == nil {
		t.Fatalf("lakefs does not expose the common authority lifecycle: %#v %v", driver, err)
	}
	base.Repository, candidate.Repository = "", ""
	if !reflect.DeepEqual(base, candidate) {
		t.Fatalf("authority selection changed fields other than repository: %#v %#v", base, candidate)
	}
	for _, retired := range []string{"gitea", "filegit"} {
		if _, err := authorityFor(retired); err == nil {
			t.Fatalf("retired Snapshot driver %s accepted", retired)
		}
	}
}

func TestAuthorityDriverNamesComeOnlyFromCompositionRegistry(t *testing.T) {
	want := []string{"lakefs"}
	if got := authorityDriverNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("formal authority registry drift: got %v want %v", got, want)
	}
}

func TestGenericAuthorityAssemblyHasNoProviderBranches(t *testing.T) {
	files := []string{
		"home_mount.go", "home_discover.go", "home.go", "stores_file.go",
		"stores_flags.go", "stores_profile.go", "stores_public.go",
	}
	fset := token.NewFileSet()
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err == nil && (value == "gitea" || value == "lakefs") {
				t.Errorf("generic authority assembly %s branches on concrete provider %q; move behavior to authority_drivers.go", path, value)
			}
			return true
		})
	}
}
