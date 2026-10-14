package cli

import (
	"encoding/json"
	declarativeindexcli "kc/declarativeindex/cli"
	discoverycli "kc/discovery/cli"
	inv "kc/invocation"
	accesscli "kc/liveaccess/cli"
	objectcli "kc/object/cli"
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
	decoder := json.NewDecoder(strings.NewReader(result.Stdout))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return result
	}
	result.Stdout = inv.JSONOut(shapeCLIProduct(path, flags, payload))
	return result
}

func shapeCLIProduct(path string, flags map[string]FlagValue, value any) any {
	if objectCommandPath(path) {
		return objectcli.Shape(path, value)
	}
	if discoveryCommandPath(path) {
		return discoverycli.Shape(path, value)
	}
	if declarativeIndexCommandPath(path) {
		return declarativeindexcli.Shape(path, value)
	}
	if liveAccessCommandPath(path) {
		return accesscli.Shape(path, value)
	}
	switch path {
	case "operations audit hitmap":
		return shapeCLIHitmap(value)
	default:
		return value
	}
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

func asObjectMap(value any) (map[string]any, bool) {
	obj, ok := value.(map[string]any)
	return obj, ok
}
