package repofile

import (
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
	"kc/kernel"
	"kc/knowledge"
)

func Serialize(address knowledge.Address, pathHint, schemaRef string, provenance *knowledge.ProvenanceEnvelope, value any) (string, error) {
	return SerializeWithSource(address, pathHint, schemaRef, nil, provenance, value)
}

func SerializeWithSource(address knowledge.Address, pathHint, schemaRef string, source *knowledge.ValueSource, provenance *knowledge.ProvenanceEnvelope, value any) (string, error) {
	fm := []string{"object_id: " + string(address.ObjectID)}
	if address.AspectName != "" {
		fm = append(fm, "aspect_name: "+address.AspectName)
	}
	if address.MemberKey != "" {
		fm = append(fm, "member_key: "+address.MemberKey)
	}
	if address.Kind != knowledge.KindEntity {
		fm = append(fm, "kind: "+string(address.Kind))
	}
	if pathHint != "" {
		fm = append(fm, "path_hint: "+pathHint)
	}
	if schemaRef != "" {
		fm = append(fm, "schema_ref: "+schemaRef)
	}
	if source = source.Normalized(); source != nil {
		b, err := json.Marshal(source)
		if err != nil {
			return "", err
		}
		fm = append(fm, "value_source: "+string(b))
	}
	if provenance != nil {
		b, err := json.Marshal(provenance)
		if err != nil {
			return "", err
		}
		fm = append(fm, "provenance: "+string(b))
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return "---\n" + strings.Join(fm, "\n") + "\n---\n" + string(body) + "\n", nil
}

func Parse(content string) *Unit {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return nil
	}
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return nil
	}
	obj := map[string]string{}
	var provenance *knowledge.ProvenanceEnvelope
	var valueSource *knowledge.ValueSource
	for _, line := range lines[1:endIdx] {
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key == "provenance" && value != "" {
			var p knowledge.ProvenanceEnvelope
			if json.Unmarshal([]byte(value), &p) == nil {
				provenance = &p
			}
			continue
		}
		if key == "value_source" && value != "" {
			var source knowledge.ValueSource
			if err := json.Unmarshal([]byte(value), &source); err != nil {
				valueSource = nil
				obj["value_source_error"] = "value_source must be valid JSON"
			} else if err := knowledge.ValidateValueSource(&source); err != nil {
				obj["value_source_error"] = err.Error()
			} else {
				valueSource = source.Normalized()
			}
			continue
		}
		obj[key] = value
	}
	objectID := obj["object_id"]
	if objectID == "" {
		return nil
	}
	body := strings.TrimSpace(strings.Join(lines[endIdx+1:], "\n"))
	var value any
	if json.Unmarshal([]byte(body), &value) != nil && looksLikeStructuredYAML(body) {
		// Knowledge drafts are commonly authored as OKF/Aspect YAML. Decode the
		// payload into the same JSON-shaped value accepted by Writer so ingest is
		// a mechanical preview, not a fixture-specific domain translation step.
		// Plain Markdown/text remains a string because it is a valid YAML scalar.
		if yaml.Unmarshal([]byte(body), &value) != nil {
			value = body
		}
	} else if value == nil && body != "null" {
		value = body
	}
	unit := &Unit{
		ObjectID: knowledge.ObjectID(objectID),
		Address:  knowledge.InferAddress(knowledge.ObjectID(objectID), obj["aspect_name"], obj["member_key"], obj["kind"]),
		PathHint: obj["path_hint"], SchemaRef: obj["schema_ref"], ValueSource: valueSource,
		Provenance: provenance, Value: value,
	}
	if message := obj["value_source_error"]; message != "" {
		unit.declarationErr = kernel.Fail(kernel.ErrUsageInvalid, "%s", message)
	}
	return unit
}

func looksLikeStructuredYAML(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return strings.HasPrefix(trimmed, "- ") || strings.Contains(trimmed, ":")
	}
	return false
}
