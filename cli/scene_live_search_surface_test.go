package cli_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// Live-only tests can otherwise keep stale setup hidden behind an environment
// skip. Their positive HTTP journeys must still use the current public scope.
func TestSceneLiveSearchEvidenceUsesAcceptedDatasetRevision(t *testing.T) {
	for _, path := range []string{"scene_accessor_notice_live_test.go", "state_search_live_test.go"} {
		t.Run(path, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			operations := map[string]int{}
			ast.Inspect(file, func(node ast.Node) bool {
				if text, ok := sceneStringLiteral(node); ok {
					switch text {
					case "/operations/v1/projections:notice", "projection.manage", "object.search", "read,search":
						t.Errorf("live search evidence retains retired positive surface %q", text)
					}
				}
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) < 4 {
					return true
				}
				fn, ok := call.Fun.(*ast.Ident)
				if !ok || fn.Name != "postAny" && fn.Name != "postAs" {
					return true
				}
				route, _ := sceneStringLiteral(call.Args[2])
				if route != "/operations/v1/projections:sync" && route != "/object/v1/search" {
					return true
				}
				operations[route]++
				if fn.Name != "postAs" {
					t.Errorf("%s must name the consumer or maintainer principal", route)
				}
				payload, ok := call.Args[len(call.Args)-1].(*ast.CompositeLit)
				if !ok {
					t.Errorf("%s must expose its fixed Dataset coordinates", route)
					return true
				}
				keys := map[string]bool{}
				for _, element := range payload.Elts {
					if pair, ok := element.(*ast.KeyValueExpr); ok {
						if key, ok := sceneStringLiteral(pair.Key); ok {
							keys[key] = true
						}
					}
				}
				if !keys["dataset"] || !keys["revision"] || keys["repository"] || keys["ref"] || keys["commit"] {
					t.Errorf("%s must use only an accepted Dataset revision: keys=%v", route, keys)
				}
				return true
			})
			if operations["/operations/v1/projections:sync"] == 0 || operations["/object/v1/search"] == 0 {
				t.Fatalf("live journey lost its explicit Dataset refresh or search: %v", operations)
			}
		})
	}
}

func sceneStringLiteral(node ast.Node) (string, bool) {
	literal, ok := node.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}
