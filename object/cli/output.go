package cli

import (
	"fmt"
	"sort"
	"strings"
)

// Shape presents Object results without losing fixed-version or maintenance evidence.
func Shape(path string, value any) any {
	switch path {
	case "read":
		return shapeCLIRead(value)
	case "resolve":
		return shapeCLIResolve(value)
	case "schema describe":
		return shapeCLISchemaDescribe(value)
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
	for _, key := range []string{"reference", "locator", "address", "digest", "declarations", "provenance"} {
		if v, exists := obj[key]; exists {
			out[key] = v
		}
	}
	if v, exists := obj["observations"]; exists {
		out["observations"] = v
	}
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
		for _, key := range []string{"definition", "schemaRef", "description"} {
			if v, exists := schema[key]; exists {
				item[key] = v
			}
		}
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
