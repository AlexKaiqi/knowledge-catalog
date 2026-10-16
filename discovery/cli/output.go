package cli

import (
	"fmt"
	"strings"
)

// Shape preserves the established query product views and raw overview facts.
func Shape(path string, value any) any {
	switch path {
	case "search":
		return shapeCLISearch(value)
	case "relations":
		return shapeCLIRelations(value)
	default:
		return value
	}
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
		if nested, ok := asObjectMap(hit["object"]); ok {
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
		for _, key := range []string{"reference", "sources"} {
			if value, exists := src[key]; exists {
				item[key] = value
			}
		}
		for _, key := range []string{"version", "evidence", "indexObservations"} {
			if value, exists := hit[key]; exists {
				item[key] = value
			}
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
	if ref, ok := asObjectMap(obj["objectRef"]); ok {
		if v := stringField(ref, "object"); v != "" {
			return v
		}
	}
	return ""
}
