package knowledge_test

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

	"kc/knowledge"
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
	raw, err := os.ReadFile(filepath.Join(root, "knowledge/testdata/system-explanations.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reviews map[knowledge.ObjectID]systemExplanationReview
	if err := json.Unmarshal(raw, &reviews); err != nil {
		t.Fatal(err)
	}
	seen := map[knowledge.ObjectID]bool{}
	for _, operation := range knowledge.SystemPublicationOperations() {
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
	if !strings.HasPrefix(basis.Owner, "docs/graph/documents/") || basis.Topic == "" || basis.Term == "" {
		t.Fatal("explanation basis must reference an existing document-graph owner and vocabulary")
	}
	raw, err := os.ReadFile(filepath.Join(root, basis.Owner))
	if err != nil {
		t.Fatal(err)
	}
	_, body, found := strings.Cut(string(raw), "\n---\n")
	if !found {
		t.Fatal("document-graph owner lacks Canonical frontmatter")
	}
	var owner struct {
		Path        string   `json:"path"`
		OwnerTopics []string `json:"ownerTopics"`
	}
	if err := json.Unmarshal([]byte(body), &owner); err != nil {
		t.Fatal(err)
	}
	owned := false
	for _, topic := range owner.OwnerTopics {
		owned = owned || topic == basis.Topic
	}
	if !owned {
		t.Fatalf("%s does not own %s", basis.Owner, basis.Topic)
	}
	document, err := os.ReadFile(filepath.Join(root, owner.Path))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(owner.Path, "docs/reviewed/") || !strings.Contains(string(document), basis.Term) {
		t.Fatal("explanation basis must refer to vocabulary in an existing reviewed owner")
	}
	if !ast.IsExported(basis.Symbol) || strings.HasSuffix(basis.Source, "_test.go") || strings.HasPrefix(basis.Source, "knowledge/system") {
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
