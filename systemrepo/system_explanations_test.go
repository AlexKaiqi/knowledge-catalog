package systemrepo_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	objectmodel "kc/object"
	"kc/systemrepo"
)

// Reading groups classify explanations, not domain types. Each explanation
// must retain the existing owners and public contracts for what it describes.
type systemExplanationReview struct {
	Path  string             `json:"path"`
	Bases []systemTopicBasis `json:"bases"`
}

type systemTopicBasis struct {
	Owner  string `json:"owner"`
	Topic  string `json:"topic"`
	Term   string `json:"term"`
	Source string `json:"source"`
	Symbol string `json:"symbol"`
}

func TestSystemExplanationsHaveExistingDesignAndPublicContracts(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(file))
	raw, err := os.ReadFile(filepath.Join(root, "systemrepo/system-explanations.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reviews map[objectmodel.ObjectID]systemExplanationReview
	if err := json.Unmarshal(raw, &reviews); err != nil {
		t.Fatal(err)
	}
	seen := map[objectmodel.ObjectID]bool{}
	for _, operation := range systemrepo.SystemPublicationOperations() {
		category, _, _ := strings.Cut(operation.PathHint, "/")
		allowed := category == "concepts" || category == "interfaces" || category == "mechanisms"
		if !allowed && operation.SchemaRef != "schema/system/explanation/v1" {
			continue
		}
		id := operation.Address.ObjectID
		review, exists := reviews[id]
		if !exists {
			t.Errorf("%s has no reviewed existing-system basis; explanations cannot create domain concepts", operation.PathHint)
			continue
		}
		if !allowed || review.Path != operation.PathHint || operation.SchemaRef != "schema/system/explanation/v1" {
			t.Errorf("%s does not match its reviewed reading category and explanation Schema", id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if len(review.Bases) == 0 {
			t.Errorf("%s has no existing public contract basis", id)
		}
		for _, basis := range review.Bases {
			t.Run(string(id)+"/"+basis.Symbol, func(t *testing.T) {
				assertSystemTopicBasis(t, root, basis)
			})
		}
	}
	for id := range reviews {
		if !seen[id] {
			t.Errorf("reviewed explanation %s is absent from the publication", id)
		}
	}
}

func assertSystemTopicBasis(t *testing.T, root string, basis systemTopicBasis) {
	t.Helper()
	if !strings.HasPrefix(basis.Owner, "doc/") || filepath.Ext(basis.Owner) != ".md" || basis.Topic == "" || basis.Term == "" {
		t.Fatal("explanation basis must reference a design document section and vocabulary")
	}
	document, err := os.ReadFile(filepath.Join(root, basis.Owner))
	if err != nil {
		t.Fatal(err)
	}
	section, found := markdownSection(string(document), basis.Topic)
	if !found {
		t.Fatalf("%s has no section %q", basis.Owner, basis.Topic)
	}
	if !strings.Contains(section, basis.Term) {
		t.Fatalf("%s section %q does not use %q", basis.Owner, basis.Topic, basis.Term)
	}
	if !ast.IsExported(basis.Symbol) || strings.HasSuffix(basis.Source, "_test.go") || strings.HasPrefix(basis.Source, "object/system") {
		t.Fatal("explanation basis must refer to a public system declaration outside the publication")
	}
	source, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, basis.Source), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !hasPublicSystemDeclaration(source, basis.Symbol) {
		t.Fatalf("%s no longer declares public %s", basis.Source, basis.Symbol)
	}
}

// markdownSection returns the heading and body of one "## " section, ignoring
// headings inside fenced code.
func markdownSection(document, heading string) (string, bool) {
	var section strings.Builder
	active, found, fenced := false, false, false
	for _, line := range strings.Split(document, "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}
		if !fenced && strings.HasPrefix(line, "## ") {
			active = strings.TrimSpace(line[3:]) == heading
			found = found || active
		}
		if active {
			section.WriteString(line)
			section.WriteByte('\n')
		}
	}
	return section.String(), found
}

func hasPublicSystemDeclaration(file *ast.File, symbol string) bool {
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if declaration.Recv == nil && declaration.Name.Name == symbol {
				return true
			}
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					if spec.Name.Name == symbol {
						return true
					}
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						if name.Name == symbol {
							return true
						}
					}
				}
			}
		}
	}
	return false
}
