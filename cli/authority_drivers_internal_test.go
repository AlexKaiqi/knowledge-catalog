package cli

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
	var normalized []StoresFile
	for _, provider := range []string{"dolt", "gitea"} {
		candidate := base
		candidate.Repository = provider
		candidate = candidate.withDefaults()
		if err := candidate.validateProfile(); err != nil {
			t.Fatalf("%s profile: %v", provider, err)
		}
		driver, err := authorityFor(candidate.Repository)
		if err != nil || driver.open == nil || driver.discover == nil {
			t.Fatalf("%s does not expose the common authority lifecycle: %#v %v", provider, driver, err)
		}
		normalized = append(normalized, candidate)
	}
	left, right := normalized[0], normalized[1]
	left.Repository, right.Repository = "", ""
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("authority switch changed fields other than repository:\nleft=%#v\nright=%#v", left, right)
	}
}

func TestAuthorityDriverNamesComeOnlyFromCompositionRegistry(t *testing.T) {
	want := []string{"dolt", "gitea"}
	if got := authorityDriverNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("formal authority registry drift: got %v want %v", got, want)
	}
}

func TestGenericAuthorityAssemblyHasNoProviderBranches(t *testing.T) {
	files := []string{
		"mount.go", "discover.go", "home.go", "stores_file.go",
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
			if err == nil && (value == "dolt" || value == "gitea") {
				t.Errorf("generic authority assembly %s branches on concrete provider %q; move behavior to authority_drivers.go", path, value)
			}
			return true
		})
	}
}
