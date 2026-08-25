package reader

import "strings"

// The parser accepts both the canonical schema object and the assembled
// single-aspect shape returned by a Repository. It has no repository access;
// schema.go owns resolution and pinned reads.
func invalidAccessTokens(value any) []string {
	seen := map[string]struct{}{}
	var walk func(any)
	walk = func(raw any) {
		switch item := raw.(type) {
		case map[string]any:
			for key, child := range item {
				if key == "access" {
					for _, token := range accessTokens(child) {
						if _, ok := knownHints[AccessHint(token)]; !ok {
							seen[token] = struct{}{}
						}
					}
					continue
				}
				walk(child)
			}
		case []any:
			for _, child := range item {
				walk(child)
			}
		}
	}
	walk(value)
	out := make([]string, 0, len(seen))
	for token := range seen {
		out = append(out, token)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func accessTokens(raw any) []string {
	var tokens []string
	switch value := raw.(type) {
	case []any:
		for _, item := range value {
			token := strings.ToLower(strings.TrimSpace(asString(item)))
			if token != "" {
				tokens = append(tokens, token)
			}
		}
	case []string:
		for _, item := range value {
			token := strings.ToLower(strings.TrimSpace(item))
			if token != "" {
				tokens = append(tokens, token)
			}
		}
	case string:
		for _, item := range strings.Fields(value) {
			tokens = append(tokens, strings.ToLower(strings.TrimSpace(item)))
		}
	}
	return tokens
}

type schemaDocument struct {
	Entity  string
	Aspect  string
	Pattern string
	Fields  []FieldAccess
}

func parseSchemaDocument(value any) schemaDocument {
	obj, ok := asObject(value)
	if !ok {
		return schemaDocument{}
	}
	if looksLikeSchema(obj) {
		return parseSchemaObject(obj)
	}
	for _, nested := range obj {
		child, ok := asObject(nested)
		if ok && looksLikeSchema(child) {
			return parseSchemaObject(child)
		}
	}
	return schemaDocument{}
}

func looksLikeSchema(obj map[string]any) bool {
	if _, ok := obj["fields"]; ok {
		return true
	}
	if _, ok := obj["pattern"]; ok {
		return true
	}
	_, hasEntity := obj["entity"]
	_, hasAspect := obj["aspect"]
	return hasEntity && hasAspect
}

func parseSchemaObject(obj map[string]any) schemaDocument {
	doc := schemaDocument{
		Entity:  stringField(obj, "entity"),
		Aspect:  stringField(obj, "aspect"),
		Pattern: normalizePattern(stringField(obj, "pattern")),
	}
	switch fields := obj["fields"].(type) {
	case map[string]any:
		for name, raw := range fields {
			doc.Fields = append(doc.Fields, parseField(name, raw))
		}
	case []any:
		for _, raw := range fields {
			item, ok := asObject(raw)
			if !ok {
				continue
			}
			name := stringField(item, "path")
			if name == "" {
				name = stringField(item, "name")
			}
			doc.Fields = append(doc.Fields, parseField(name, raw))
		}
	}
	return doc
}

func parseField(path string, raw any) FieldAccess {
	field := FieldAccess{Path: path, Access: []AccessHint{}}
	obj, ok := asObject(raw)
	if !ok {
		return field
	}
	if p := stringField(obj, "path"); p != "" {
		field.Path = p
	} else if n := stringField(obj, "name"); n != "" && field.Path == "" {
		field.Path = n
	}
	field.Type = stringField(obj, "type")
	field.Access = normalizeHints(obj["access"])
	return field
}

func normalizeHints(raw any) []AccessHint {
	tokens := accessTokens(raw)
	seen := map[AccessHint]struct{}{}
	for _, token := range tokens {
		if token == "" {
			continue
		}
		hint := AccessHint(token)
		if _, ok := knownHints[hint]; !ok {
			continue
		}
		seen[hint] = struct{}{}
	}
	out := []AccessHint{}
	for _, hint := range hintOrder {
		if _, ok := seen[hint]; ok {
			out = append(out, hint)
		}
	}
	return out
}

func normalizePattern(raw string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(raw)), " ", "_")
}

func asObject(value any) (map[string]any, bool) {
	obj, ok := value.(map[string]any)
	return obj, ok
}

func stringField(obj map[string]any, key string) string {
	return strings.TrimSpace(asString(obj[key]))
}

func asString(value any) string {
	s, _ := value.(string)
	return s
}

func sortFieldAccess(fields []FieldAccess) {
	for i := 0; i < len(fields); i++ {
		for j := i + 1; j < len(fields); j++ {
			if fields[j].Path < fields[i].Path {
				fields[i], fields[j] = fields[j], fields[i]
			}
		}
	}
}
