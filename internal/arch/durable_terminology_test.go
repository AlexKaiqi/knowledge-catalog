package arch_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// A historical wire spelling is allowed only in its registered constant. This
// does not exempt its file, other literals or public configuration vocabulary.
func durableTerminologyContext(relative, text, retired string) string {
	if relative != "home/managed_legacy_actions.go" {
		return text
	}
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, relative, text, 0)
	if err != nil {
		return text
	}
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}
		for _, specification := range group.Specs {
			value, ok := specification.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != "managedLegacyActionPrefix" || len(value.Values) != 1 {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				continue
			}
			decoded, err := strconv.Unquote(literal.Value)
			if err != nil || decoded != retired+"." {
				continue
			}
			start, end := positions.Position(literal.Pos()).Offset, positions.Position(literal.End()).Offset
			return text[:start] + `"historical-action."` + text[end:]
		}
	}
	return text
}
