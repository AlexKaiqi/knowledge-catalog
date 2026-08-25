package index

import (
	"encoding/json"
	"strings"

	"kc/knowledge"
	"kc/reader"
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

// boundSpec is the AccessSpec that applies to one object: recipe objects contribute
// nothing; schema_ref selects which declared schemas; otherwise the full spec.
func boundSpec(repo knowledge.Repository, value knowledge.KnowledgeValue, spec reader.AccessSpec) reader.AccessSpec {
	if knowledge.IsSchemaObject(value.Address.ObjectID) {
		return reader.AccessSpec{AccessDigest: spec.AccessDigest}
	}
	return spec.Bind(objectSchemaRefs(repo, value))
}

func objectSchemaRefs(repo knowledge.Repository, value knowledge.KnowledgeValue) []string {
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

func documentText(value knowledge.KnowledgeValue, spec reader.AccessSpec) string {
	assembled := value.Value
	var parts []string
	for _, field := range spec.Fields {
		if !field.Has(reader.HintText) {
			continue
		}
		if v, ok := fieldValue(assembled, field.Aspect, field.Path); ok {
			values := []any{v}
			if list, ok := v.([]any); ok {
				values = list
			}
			for _, item := range values {
				if s := scalarString(item); s != "" {
					parts = append(parts, s)
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

func indexedFields(value knowledge.KnowledgeValue, spec reader.AccessSpec) [][2]string {
	assembled := value.Value
	var out [][2]string
	seen := map[string]struct{}{}
	for _, field := range spec.Fields {
		if !field.Has(reader.HintFilter) && !field.Has(reader.HintSort) && !field.Has(reader.HintText) {
			continue
		}
		v, ok := fieldValue(assembled, field.Aspect, field.Path)
		if !ok {
			continue
		}
		key := field.FieldRef.Key()
		values := []any{v}
		if list, ok := v.([]any); ok {
			values = list
		}
		for _, item := range values {
			s, valid := reader.NormalizeScalarValue(field.Type, item)
			if !valid {
				continue
			}
			identity := key + "\x00" + s
			if _, dup := seen[identity]; dup {
				continue
			}
			seen[identity] = struct{}{}
			out = append(out, [2]string{key, s})
		}
	}
	return out
}
