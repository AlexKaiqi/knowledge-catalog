package index

import (
	"encoding/json"
	"strings"

	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

func lookupPath(value any, path string) (any, bool) {
	if path == "" {
		return value, value != nil
	}
	cur := value
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func fieldValue(assembled any, aspect, path string) (any, bool) {
	if aspect != "" {
		if obj, ok := assembled.(map[string]any); ok {
			if inner, ok := obj[aspect]; ok {
				if v, ok := lookupPath(inner, path); ok {
					return v, true
				}
			}
		}
	}
	return lookupPath(assembled, path)
}

func scalarString(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(b)
}

// boundSpec is the IndexSpec that applies to one object: recipe objects contribute
// nothing; schema_ref selects which declared schemas; otherwise the full spec.
func boundSpec(repo repository.Repository, value repository.KnowledgeValue, spec reader.IndexSpec) reader.IndexSpec {
	if kernel.IsSchemaObject(value.Address.ObjectID) {
		return reader.IndexSpec{Digest: spec.Digest}
	}
	return spec.Bind(objectSchemaRefs(repo, value))
}

func objectSchemaRefs(repo repository.Repository, value repository.KnowledgeValue) []string {
	if repo == nil {
		return nil
	}
	if len(value.Units) == 0 {
		res, err := repo.Resolve(value.Address.ObjectID, value.Commit)
		if err != nil || res.SchemaRef == "" {
			return nil
		}
		return []string{res.SchemaRef}
	}
	var refs []string
	seen := map[string]struct{}{}
	for _, addr := range value.Units {
		res, err := repo.ResolveAddress(addr, value.Commit)
		if err != nil || res.SchemaRef == "" {
			continue
		}
		if _, ok := seen[res.SchemaRef]; ok {
			continue
		}
		seen[res.SchemaRef] = struct{}{}
		refs = append(refs, res.SchemaRef)
	}
	return refs
}

func documentText(value repository.KnowledgeValue, spec reader.IndexSpec) string {
	assembled := value.Value
	var parts []string
	for _, field := range spec.Fields {
		if !field.Has(reader.HintText) && !field.Has(reader.HintSummary) {
			continue
		}
		if v, ok := fieldValue(assembled, field.Aspect, field.Path); ok {
			if s := scalarString(v); s != "" {
				parts = append(parts, s)
			}
		}
	}
	return strings.Join(parts, " ")
}

func indexedFields(value repository.KnowledgeValue, spec reader.IndexSpec) [][2]string {
	assembled := value.Value
	var out [][2]string
	for _, field := range spec.Fields {
		if !field.Has(reader.HintKey) && !field.Has(reader.HintFilter) && !field.Has(reader.HintSort) && !field.Has(reader.HintText) && !field.Has(reader.HintSummary) {
			continue
		}
		v, ok := fieldValue(assembled, field.Aspect, field.Path)
		if !ok {
			continue
		}
		s := scalarString(v)
		if s == "" {
			continue
		}
		out = append(out, [2]string{field.Path, s})
	}
	return out
}
