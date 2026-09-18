package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// encodeCLIProduct reshapes successful CLI JSON so stdout answers the command's
// human question. HTTP and the shared executor keep protocol DTOs (API-01).
func encodeCLIProduct(path string, flags map[string]FlagValue, result RunResult) RunResult {
	if result.Status != 0 && result.Status != 2 {
		return result
	}
	trimmed := strings.TrimSpace(result.Stdout)
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
		return result
	}
	var payload any
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		return result
	}
	result.Stdout = jsonOut(shapeCLIProduct(path, flags, payload))
	return result
}

func shapeCLIProduct(path string, flags map[string]FlagValue, value any) any {
	switch path {
	case "read":
		return shapeCLIRead(value)
	case "search":
		return shapeCLISearch(value)
	case "grant list":
		return shapeCLIGrantList(value, flags)
	case "schema describe":
		return shapeCLISchemaDescribe(value)
	case "resolve":
		return shapeCLIResolve(value)
	case "relations":
		return shapeCLIRelations(value)
	case "operations audit hitmap":
		return shapeCLIHitmap(value)
	case "access":
		return shapeCLIAccess(value)
	default:
		return value
	}
}

func shapeCLIRead(value any) any {
	if items, ok := value.([]any); ok {
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, shapeCLIReadObject(item))
		}
		return out
	}
	return shapeCLIReadObject(value)
}

func shapeCLIReadObject(value any) any {
	obj, ok := asObjectMap(value)
	if !ok {
		return value
	}
	out := map[string]any{}
	if v := stringField(obj, "repository"); v != "" {
		out["repository"] = v
	}
	if v := productObjectID(obj); v != "" {
		out["objectId"] = v
	}
	if v := stringField(obj, "commit"); v != "" {
		out["commit"] = v
	}
	if v, exists := obj["value"]; exists {
		out["value"] = v
	}
	if v := productAspectName(obj); v != "" {
		out["aspectName"] = v
	}
	if v := productSchemaRef(obj); v != "" {
		out["schemaRef"] = v
	}
	return out
}

func shapeCLISearch(value any) any {
	root, ok := asObjectMap(value)
	if !ok {
		return value
	}
	hitsIn, _ := root["hits"].([]any)
	hits := make([]any, 0, len(hitsIn))
	for _, raw := range hitsIn {
		hit, ok := asObjectMap(raw)
		if !ok {
			continue
		}
		src := hit
		if nested, ok := asObjectMap(hit["knowledge"]); ok {
			src = nested
		}
		item := map[string]any{}
		if v := stringField(src, "repository"); v != "" {
			item["repository"] = v
		}
		if v := productObjectID(src); v != "" {
			item["objectId"] = v
		}
		if v := stringField(src, "commit"); v != "" {
			item["commit"] = v
		}
		hits = append(hits, item)
	}
	out := map[string]any{}
	for key, v := range root {
		if key == "hits" {
			continue
		}
		out[key] = v
	}
	out["hits"] = hits
	return out
}

func shapeCLISchemaDescribe(value any) any {
	if items, ok := value.([]any); ok {
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, shapeCLISchemaDescribeReport(item))
		}
		return out
	}
	return shapeCLISchemaDescribeReport(value)
}

func shapeCLISchemaDescribeReport(value any) any {
	obj, ok := asObjectMap(value)
	if !ok {
		return value
	}
	out := map[string]any{}
	if v := stringField(obj, "repository"); v != "" {
		out["repository"] = v
	}
	if v := stringField(obj, "commit"); v != "" {
		out["commit"] = v
	}
	schemasIn, _ := obj["schemas"].([]any)
	schemas := make([]any, 0, len(schemasIn))
	for _, raw := range schemasIn {
		schema, ok := asObjectMap(raw)
		if !ok {
			continue
		}
		item := map[string]any{}
		if v := stringField(schema, "objectId"); v != "" {
			item["objectId"] = v
		}
		if v := stringField(schema, "origin"); v != "" {
			item["origin"] = v
		}
		fieldsIn, _ := schema["fields"].([]any)
		fields := make([]any, 0, len(fieldsIn))
		for _, fieldRaw := range fieldsIn {
			field, ok := asObjectMap(fieldRaw)
			if !ok {
				continue
			}
			shaped := map[string]any{}
			if v := stringField(field, "path"); v != "" {
				shaped["path"] = v
			}
			if v := stringField(field, "type"); v != "" {
				shaped["type"] = v
			}
			if access, exists := field["access"]; exists {
				shaped["access"] = access
			}
			fields = append(fields, shaped)
		}
		sort.Slice(fields, func(i, j int) bool {
			left, _ := asObjectMap(fields[i])
			right, _ := asObjectMap(fields[j])
			return stringField(left, "path") < stringField(right, "path")
		})
		item["fields"] = fields
		schemas = append(schemas, item)
	}
	out["schemas"] = schemas
	return out
}

func shapeCLIAccess(value any) any {
	obj, ok := asObjectMap(value)
	if !ok {
		return value
	}
	observations, _ := obj["observations"].([]any)
	if len(observations) == 0 {
		return value
	}
	first, ok := asObjectMap(observations[0])
	if !ok {
		return value
	}
	out := map[string]any{}
	if value, exists := first["value"]; exists {
		out["value"] = value
	}
	if basis, exists := first["basis"]; exists {
		out["basis"] = basis
	}
	bindings, _ := obj["bindings"].([]any)
	if len(bindings) > 0 {
		if binding, ok := asObjectMap(bindings[0]); ok {
			if v := stringField(binding, "schemaRef"); v != "" {
				out["schemaRef"] = v
			}
			if address, ok := asObjectMap(binding["address"]); ok {
				if v := stringField(address, "objectId"); v != "" {
					out["objectId"] = v
				}
				if v := stringField(address, "aspectName"); v != "" {
					out["aspectName"] = v
				}
			}
		}
	}
	return out
}

func shapeCLIResolve(value any) any {
	if items, ok := value.([]any); ok {
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, shapeCLIResolveObject(item))
		}
		return out
	}
	return shapeCLIResolveObject(value)
}

func shapeCLIResolveObject(value any) any {
	obj, ok := asObjectMap(value)
	if !ok {
		return value
	}
	out := map[string]any{}
	if v := stringField(obj, "repository"); v != "" {
		out["repository"] = v
	}
	if v := productObjectID(obj); v != "" {
		out["objectId"] = v
	}
	if v := stringField(obj, "commit"); v != "" {
		out["commit"] = v
	}
	if v := stringField(obj, "status"); v != "" {
		out["status"] = v
	}
	if v := stringField(obj, "digest"); v != "" {
		out["digest"] = v
	}
	if v := productAspectName(obj); v != "" {
		out["aspectName"] = v
	}
	if v := productMemberKey(obj); v != "" {
		out["memberKey"] = v
	}
	return out
}

func shapeCLIRelations(value any) any {
	root, ok := asObjectMap(value)
	if !ok {
		return value
	}
	hitsIn, _ := root["hits"].([]any)
	hits := make([]any, 0, len(hitsIn))
	for _, raw := range hitsIn {
		hit, ok := asObjectMap(raw)
		if !ok {
			continue
		}
		item := map[string]any{}
		if v := stringField(hit, "repository"); v != "" {
			item["repository"] = v
		}
		if v := productObjectID(hit); v != "" {
			item["objectId"] = v
		}
		if v := stringField(hit, "commit"); v != "" {
			item["commit"] = v
		}
		if relation, ok := asObjectMap(hit["relation"]); ok {
			if v := stringField(relation, "relationType"); v != "" {
				item["relationType"] = v
			}
		}
		if roles, ok := hit["matchedRoles"]; ok {
			item["matchedRoles"] = roles
		}
		hits = append(hits, item)
	}
	out := map[string]any{}
	for key, v := range root {
		if key == "hits" || key == "searchView" {
			continue
		}
		out[key] = v
	}
	out["hits"] = hits
	return out
}

func shapeCLIHitmap(value any) any {
	root, ok := asObjectMap(value)
	if !ok {
		return value
	}
	out := map[string]any{}
	for key, v := range root {
		if key == "source" {
			continue
		}
		out[key] = v
	}
	out["source"] = "hitmap"
	return out
}

func shapeCLIGrantList(value any, flags map[string]FlagValue) any {
	root, ok := asObjectMap(value)
	if !ok {
		return value
	}
	if _, eval := root["allow"]; eval {
		return value
	}
	rulesIn, ok := root["rules"].([]any)
	if !ok {
		return value
	}
	repo := ""
	catalog := ""
	if flags != nil {
		repo = FlagString(flags, "repo")
		catalog = FlagString(flags, "catalog")
	}
	rules := make([]any, 0, len(rulesIn))
	for _, raw := range rulesIn {
		rule, ok := asObjectMap(raw)
		if !ok {
			continue
		}
		if repo != "" && stringField(rule, "repo") != repo {
			continue
		}
		if catalog != "" && stringField(rule, "catalog") != catalog {
			continue
		}
		rules = append(rules, raw)
	}
	return map[string]any{"rules": rules}
}

func asObjectMap(value any) (map[string]any, bool) {
	obj, ok := value.(map[string]any)
	return obj, ok
}

func stringField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	v, ok := obj[key]
	if !ok || v == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "<nil>" {
		return ""
	}
	return s
}

func productObjectID(obj map[string]any) string {
	if v := stringField(obj, "objectId"); v != "" {
		return v
	}
	if address, ok := asObjectMap(obj["address"]); ok {
		if v := stringField(address, "objectId"); v != "" {
			return v
		}
	}
	if ref, ok := asObjectMap(obj["knowledgeRef"]); ok {
		if v := stringField(ref, "object"); v != "" {
			return v
		}
	}
	return ""
}

func productAspectName(obj map[string]any) string {
	if v := stringField(obj, "aspectName"); v != "" {
		return v
	}
	if address, ok := asObjectMap(obj["address"]); ok {
		return stringField(address, "aspectName")
	}
	return ""
}

func productMemberKey(obj map[string]any) string {
	if v := stringField(obj, "memberKey"); v != "" {
		return v
	}
	if address, ok := asObjectMap(obj["address"]); ok {
		return stringField(address, "memberKey")
	}
	return ""
}

func productSchemaRef(obj map[string]any) string {
	if v := stringField(obj, "schemaRef"); v != "" {
		return v
	}
	decls, _ := obj["declarations"].([]any)
	for _, raw := range decls {
		decl, ok := asObjectMap(raw)
		if !ok {
			continue
		}
		if v := stringField(decl, "schemaRef"); v != "" {
			return v
		}
	}
	return ""
}
