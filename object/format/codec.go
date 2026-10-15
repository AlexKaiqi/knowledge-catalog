package format

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"kc/kernel"
	objectmodel "kc/object"
)

func Serialize(address objectmodel.Address, pathHint, schemaRef string, provenance *objectmodel.ProvenanceEnvelope, value any) (string, error) {
	return SerializeWithSource(address, pathHint, schemaRef, nil, provenance, value)
}

func SerializeWithSource(address objectmodel.Address, pathHint, schemaRef string, source *objectmodel.ValueSource, provenance *objectmodel.ProvenanceEnvelope, value any) (string, error) {
	bodyValue := value
	markdown, markdownBody := markdownAspectBody(address, pathHint, schemaRef, bodyValue)
	var fm []string
	if markdown {
		fm = []string{"entity: " + string(address.ObjectID)}
		if address.AspectName != "" {
			fm = append(fm, "aspect: "+address.AspectName)
		}
	} else {
		fm = []string{"object_id: " + string(address.ObjectID)}
		if address.AspectName != "" {
			fm = append(fm, "aspect_name: "+address.AspectName)
		}
	}
	if address.MemberKey != "" {
		fm = append(fm, "member_key: "+address.MemberKey)
	}
	if !markdown && address.Kind != objectmodel.KindEntity {
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
		fm = append(fm, "valueSource: "+string(b))
	}
	if provenance != nil {
		b, err := json.Marshal(provenance)
		if err != nil {
			return "", err
		}
		fm = append(fm, "provenance: "+string(b))
	}
	if markdown {
		return "---\n" + strings.Join(fm, "\n") + "\n---\n" + markdownBody + "\n", nil
	}
	if source != nil && bodyValue == nil {
		return "---\n" + strings.Join(fm, "\n") + "\n---\n", nil
	}
	body, err := json.MarshalIndent(bodyValue, "", "  ")
	if err != nil {
		return "", err
	}
	return "---\n" + strings.Join(fm, "\n") + "\n---\n" + string(body) + "\n", nil
}

func markdownAspectBody(address objectmodel.Address, pathHint, schemaRef string, value any) (bool, string) {
	if !objectmodel.IsReadmeAddress(address, schemaRef) {
		if !strings.HasSuffix(strings.ToLower(pathHint), ".md") {
			return false, ""
		}
		// A path alone cannot reinterpret a scalar or discard other fields.
		// Generic Markdown units explicitly carry the single body field.
		record, ok := value.(map[string]any)
		if !ok || len(record) != 1 {
			return false, ""
		}
		if _, ok := record["body"].(string); !ok {
			return false, ""
		}
	}
	body, ok := objectmodel.ReadmeBody(value)
	if !ok {
		return false, ""
	}
	return true, body
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
	var provenance *objectmodel.ProvenanceEnvelope
	var valueSource *objectmodel.ValueSource
	header := lines[1:endIdx]
	for i := 0; i < len(header); i++ {
		line := header[i]
		idx := strings.Index(line, ":")
		if idx < 0 || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key == "provenance" && value != "" {
			var p objectmodel.ProvenanceEnvelope
			if json.Unmarshal([]byte(value), &p) == nil {
				provenance = &p
			}
			continue
		}
		if key == "valueSource" {
			block := []string{line}
			for i+1 < len(header) && (strings.HasPrefix(header[i+1], " ") || strings.HasPrefix(header[i+1], "\t")) {
				i++
				block = append(block, header[i])
			}
			source, err := decodeFrontMatterValueSource(strings.Join(block, "\n"))
			if err != nil {
				obj["value_source_error"] = err.Error()
			} else {
				valueSource = source
			}
			continue
		}
		obj[key] = value
	}
	objectID := obj["object_id"]
	if objectID == "" {
		objectID = obj["entity"]
	}
	if objectID == "" {
		return nil
	}
	aspectName := obj["aspect_name"]
	if aspectName == "" {
		aspectName = obj["aspect"]
	}
	body := strings.TrimSpace(strings.Join(lines[endIdx+1:], "\n"))
	schemaRef := obj["schema_ref"]
	var value any
	if valueSource != nil && body == "" {
		// An external unit's header is its whole content.
	} else if objectmodel.IsReadmeSchema(schemaRef) || (obj["entity"] != "" && strings.HasSuffix(strings.ToLower(obj["path_hint"]), ".md")) {
		value = map[string]any{"body": body}
	} else if kernel.UnmarshalJSON([]byte(body), &value) != nil && looksLikeStructuredYAML(body) {
		// Object drafts are commonly authored as YAML. Decode the
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
		ObjectID: objectmodel.ObjectID(objectID),
		Address:  objectmodel.InferAddress(objectmodel.ObjectID(objectID), aspectName, obj["member_key"], obj["kind"]),
		PathHint: obj["path_hint"], SchemaRef: schemaRef, ValueSource: valueSource,
		Provenance: provenance, Value: value,
	}
	if _, declared := obj["origin"]; declared {
		unit.declarationErr = kernel.Fail(kernel.ErrUsageInvalid, "origin is not a unit header; it belongs to a ResourceDescriptor referenced by valueSource")
	}
	if _, declared := obj["value_source"]; declared {
		unit.declarationErr = kernel.Fail(kernel.ErrUsageInvalid, "value_source is retired; declare valueSource: {kind: binding, binding: {descriptorRef: <id>}}")
	}
	if message := obj["value_source_error"]; message != "" {
		unit.declarationErr = kernel.Fail(kernel.ErrUsageInvalid, "%s", message)
	}
	return unit
}

// decodeFrontMatterValueSource reads `valueSource:` written either as one
// JSON/flow line or as a nested YAML block, then applies the strict JSON shape.
func decodeFrontMatterValueSource(block string) (*objectmodel.ValueSource, error) {
	var header map[string]any
	if err := yaml.Unmarshal([]byte(block), &header); err != nil {
		return nil, fmt.Errorf("valueSource must be a YAML or JSON mapping")
	}
	raw, err := json.Marshal(header["valueSource"])
	if err != nil {
		return nil, fmt.Errorf("valueSource must be a mapping")
	}
	var source objectmodel.ValueSource
	if err := json.Unmarshal(raw, &source); err != nil {
		return nil, err
	}
	if err := objectmodel.ValidateValueSource(&source); err != nil {
		return nil, err
	}
	return source.Normalized(), nil
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
