package discovery

import (
	"context"
	"fmt"
	"sort"
	"strings"

	objectmodel "kc/object"
	"kc/object/reader"
)

func memberFieldKey(field FieldRef, member string) string {
	if member == "" {
		return field.Key()
	}
	return member + "\x1e" + field.Key()
}

func constructDeclaredFields(ctx context.Context, source FieldSet, spec AccessSpec, cache MemberComputations) (FieldSet, error) {
	out := FieldSet{Kind: source.Kind}
	eligible := map[string]bool{}
	texts := []string{}
	for _, build := range spec.builds {
		values, applies, excluded := declarationValues(source, build)
		if !applies || excluded {
			continue
		}
		if err := ctx.Err(); err != nil {
			return FieldSet{}, err
		}
		constructed, err := constructMember(ctx, build, values, cache)
		if err != nil {
			return FieldSet{}, err
		}
		member := objectmodel.AddressKey(build.member)
		if build.declaration.Build.Kind == "vector" {
			text := constructed.Text
			model, _ := build.declaration.Build.Options["model"].(string)
			if strings.TrimSpace(text) != "" && model != "" {
				out.Semantic = append(out.Semantic, SemanticField{Member: member, Model: model, Text: text})
			}
			continue
		}
		if build.declaration.Build.Kind == "text" {
			text := constructed.Text
			if text == "" {
				continue
			}
			texts = append(texts, text)
			for _, input := range build.inputs {
				field, err := spec.ResolveField(input.Field)
				if err != nil {
					return FieldSet{}, err
				}
				field.Access = []reader.AccessHint{reader.HintText}
				out.Values = append(out.Values, FieldValue{Field: field, Value: text, Member: member})
			}
			continue
		}
		for i, input := range build.inputs {
			field, err := spec.ResolveField(input.Field)
			if err != nil {
				return FieldSet{}, err
			}
			field.Access = declarationHints(build.declaration.Uses)
			if build.declaration.Build.Kind == "keyword" {
				field.Type = "string"
			}
			if field.Has(reader.HintFilter) {
				eligible[memberFieldKey(field.FieldRef, member)] = true
			}
			for _, value := range constructed.Values[i] {
				out.Values = append(out.Values, FieldValue{Field: field, Value: value, Member: member})
			}
		}
	}
	for key := range eligible {
		out.EligibleFields = append(out.EligibleFields, key)
	}
	sort.Strings(out.EligibleFields)
	sort.Slice(out.Values, func(i, j int) bool {
		if out.Values[i].Key() != out.Values[j].Key() {
			return out.Values[i].Key() < out.Values[j].Key()
		}
		return out.Values[i].Value < out.Values[j].Value
	})
	out.Text = strings.Join(texts, " ")
	return out, nil
}

func declarationValues(source FieldSet, build declarationBuild) ([][]string, bool, bool) {
	values := make([][]string, len(build.inputs))
	applies, excluded := false, false
	for i, input := range build.inputs {
		for key := range source.applicable {
			form, path, _ := strings.Cut(key, "\x1f")
			if declarationInputApplies(input, form, path) {
				applies = true
			}
		}
		for _, cell := range source.Values {
			if cell.Field.FieldRef == input.Field && declarationInputApplies(input, cell.form, cell.Field.Path) {
				values[i] = append(values[i], cell.Value)
			}
		}
		if len(values[i]) == 0 && build.declaration.Inputs[i].Missing == objectmodel.IndexMissingExclude {
			excluded = true
		}
	}
	return values, applies, excluded
}

func declarationInputApplies(input DeclarationInput, form, path string) bool {
	if path != input.Field.Path {
		return false
	}
	if strings.Contains(input.form, "@") {
		return form == input.form
	}
	current, _, _ := strings.Cut(form, "@")
	return current == input.form
}

func declarationHints(uses []string) []reader.AccessHint {
	var hints []reader.AccessHint
	for _, use := range uses {
		if hint, scalar := useHint(use); scalar {
			hints = appendHint(hints, hint)
		}
	}
	return hints
}

func declarationScalar(declaration objectmodel.IndexDeclaration, value string) string {
	if declaration.Build.Kind == "keyword" && declaration.Build.Options["normalizer"] == "lowercase" {
		return strings.ToLower(value)
	}
	return value
}

func declarationText(declaration objectmodel.IndexDeclaration, values [][]string) (string, error) {
	parts := make([]string, len(values))
	for i, input := range values {
		parts[i] = strings.Join(input, " ")
	}
	template, present := declaration.Build.Options["template"]
	if !present {
		return strings.Join(parts, " "), nil
	}
	text, ok := template.(string)
	if !ok {
		return "", fmt.Errorf("template must be a string")
	}
	return indexTemplate(text, declaration.Inputs, parts)
}

// CheckIndexTemplate checks the same placeholders used by physical construction.
// Placeholders name an input's Schema field path; literal text is preserved.
func CheckIndexTemplate(template string, inputs []objectmodel.IndexInput) string {
	_, err := indexTemplate(template, inputs, make([]string, len(inputs)))
	if err != nil {
		return err.Error()
	}
	return ""
}

func indexTemplate(template string, inputs []objectmodel.IndexInput, values []string) (string, error) {
	bindings := map[string]int{}
	for i, input := range inputs {
		if _, exists := bindings[input.Locator.Path]; exists {
			bindings[input.Locator.Path] = -1
		} else {
			bindings[input.Locator.Path] = i
		}
	}
	var out strings.Builder
	for len(template) > 0 {
		start := strings.IndexAny(template, "{}")
		if start < 0 {
			out.WriteString(template)
			break
		}
		out.WriteString(template[:start])
		if template[start] != '{' {
			return "", fmt.Errorf("template has an unmatched closing brace")
		}
		end := strings.IndexByte(template[start+1:], '}')
		if end < 0 {
			return "", fmt.Errorf("template has an unclosed placeholder")
		}
		end += start + 1
		name := template[start+1 : end]
		position, exists := bindings[name]
		if !exists {
			return "", fmt.Errorf("template placeholder %q does not name an input field", name)
		}
		if position < 0 {
			return "", fmt.Errorf("template placeholder %q is ambiguous across inputs", name)
		}
		out.WriteString(values[position])
		template = template[end+1:]
	}
	return out.String(), nil
}
