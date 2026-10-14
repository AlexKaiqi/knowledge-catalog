package cli

// Shape preserves the established first-observation access presentation. The
// typed API still returns all bindings and observations unchanged.
func Shape(path string, value any) any {
	if path != "access" {
		return value
	}
	root, ok := value.(map[string]any)
	if !ok {
		return value
	}
	observations, _ := root["observations"].([]any)
	if len(observations) == 0 {
		return value
	}
	first, ok := observations[0].(map[string]any)
	if !ok {
		return value
	}
	out := map[string]any{}
	for _, key := range []string{"value", "basis"} {
		if observed, present := first[key]; present {
			out[key] = observed
		}
	}
	bindings, _ := root["bindings"].([]any)
	if len(bindings) == 0 {
		return out
	}
	binding, _ := bindings[0].(map[string]any)
	if schema, ok := binding["schemaRef"].(string); ok && schema != "" {
		out["schemaRef"] = schema
	}
	address, _ := binding["address"].(map[string]any)
	for _, key := range []string{"objectId", "aspectName"} {
		if coordinate, ok := address[key].(string); ok && coordinate != "" {
			out[key] = coordinate
		}
	}
	return out
}
