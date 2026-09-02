package repofile

import (
	"regexp"
	"strings"

	"kc/internal/treepath"
	"kc/kernel"
	"kc/knowledge"
)

var knowledgeFile = regexp.MustCompile(`(?i)\.(json|md|okf|ya?ml|txt)$`)

// KnowledgePath is true when a tree path may hold a knowledge unit.
func KnowledgePath(rel string) bool {
	return knowledgeFile.MatchString(rel)
}

func DefaultPath(address knowledge.Address, schemaRef string) string {
	if knowledge.IsSchemaObject(address.ObjectID) && address.AspectName == "" && address.MemberKey == "" {
		return DefaultSchemaPath(address.ObjectID)
	}
	rel := instanceRelativePath(address)
	if dir := InstanceTypeDir(schemaRef); dir != "" {
		return dir + "/" + rel
	}
	return rel
}

func instanceRelativePath(address knowledge.Address) string {
	if address.MemberKey != "" && address.AspectName != "" {
		return string(address.ObjectID) + "/" + address.AspectName + "/" + address.MemberKey + ".json"
	}
	if address.AspectName != "" {
		return string(address.ObjectID) + "/" + address.AspectName + ".json"
	}
	return string(address.ObjectID) + ".json"
}

// InstanceTypeDir is the Canonical type folder for an instance, derived from
// schema_ref (schema/metric.properties → metrics). Domain Schema stays in schemas/.
func InstanceTypeDir(schemaRef string) string {
	parsed, ok := knowledge.ParseSchemaRef(schemaRef)
	if !ok || !knowledge.IsSchemaObject(parsed.Object) {
		return ""
	}
	rest := strings.TrimPrefix(string(parsed.Object), knowledge.SchemaObjectPrefix)
	entity := rest
	if i := strings.IndexByte(rest, '.'); i > 0 {
		entity = rest[:i]
	} else {
		parts := strings.Split(rest, "/")
		switch {
		case len(parts) >= 2 && (parts[0] == "core" || parts[0] == "meta"):
			entity = parts[1]
		case len(parts) > 0:
			entity = parts[0]
		}
	}
	return pluralizeType(entity)
}

func pluralizeType(entity string) string {
	switch entity {
	case "":
		return ""
	case "relation":
		return "relations"
	case "resource-descriptor":
		return "resources"
	}
	if strings.HasSuffix(entity, "s") {
		return entity
	}
	return entity + "s"
}

// DefaultSchemaPath is the Canonical tree path for a schema/* object.
// All Domain Schema files sit in one schemas/ directory; identity remains object_id.
func DefaultSchemaPath(objectID knowledge.ObjectID) string {
	rest := strings.TrimPrefix(string(objectID), knowledge.SchemaObjectPrefix)
	if rest == "" || rest == string(objectID) {
		return "schemas/" + string(objectID) + ".aspect.yaml"
	}
	parts := strings.Split(rest, "/")
	if n := len(parts); n >= 2 && isSchemaVersionSegment(parts[n-1]) {
		return "schemas/" + parts[n-2] + "." + parts[n-1] + ".aspect.yaml"
	}
	return "schemas/" + rest + ".aspect.yaml"
}

func isSchemaVersionSegment(value string) bool {
	if len(value) < 2 || value[0] != 'v' {
		return false
	}
	for i := 1; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

// PathHintForIngest chooses the stored tree path for one ingested unit.
// schema/* always lands in the single schemas/ directory.
func PathHintForIngest(address knowledge.Address, declared, rel string) string {
	if knowledge.IsSchemaObject(address.ObjectID) {
		return DefaultSchemaPath(address.ObjectID)
	}
	hint := strings.TrimSpace(declared)
	if hint == "" {
		hint = rel
	}
	if hint == "" {
		return DefaultPath(address, "")
	}
	return hint
}

func SafeRelativePath(value string) (string, error) {
	return treepath.Clean(value)
}

func EntityPathHint(units []Unit, objectID knowledge.ObjectID) string {
	if len(units) == 1 {
		if units[0].PathHint != "" {
			return units[0].PathHint
		}
		return units[0].Path
	}
	if len(units) == 0 {
		return ""
	}
	return string(objectID)
}

func AssertLayout(units []Unit, incoming knowledge.Address) error {
	if len(units) == 0 {
		return nil
	}
	hasBlob, hasAspect := false, false
	for _, u := range units {
		if knowledge.IsEntityBlob(u.Address) {
			hasBlob = true
		} else {
			hasAspect = true
		}
	}
	if hasBlob && hasAspect {
		return kernel.Fail(kernel.ErrObjectIDConflict, "%s mixes entity blob and aspects", incoming.ObjectID)
	}
	if knowledge.IsEntityBlob(incoming) && hasAspect {
		return kernel.Fail(kernel.ErrObjectIDConflict, "cannot PUT an entity blob on %s; object already has aspects", incoming.ObjectID)
	}
	if !knowledge.IsEntityBlob(incoming) && hasBlob {
		return kernel.Fail(kernel.ErrObjectIDConflict, "cannot PUT an aspect on %s; object is an entity blob", incoming.ObjectID)
	}
	return nil
}
