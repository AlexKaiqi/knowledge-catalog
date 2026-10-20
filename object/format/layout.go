package format

import (
	"regexp"
	"strings"

	"kc/internal/treepath"
	objectmodel "kc/object"
)

var objectFile = regexp.MustCompile(`(?i)\.(json|md|okf|ya?ml|txt)$`)

// ObjectPath is true when a tree path may hold an object unit.
func ObjectPath(rel string) bool {
	return objectFile.MatchString(rel)
}

func DefaultPath(address objectmodel.Address, schemaRef string) string {
	if objectmodel.IsReadmeAddress(address, schemaRef) {
		return objectmodel.RepositoryReadmePath
	}
	if objectmodel.IsSchemaObject(address.ObjectID) && address.AspectName == "" && address.MemberKey == "" {
		return DefaultSchemaPath(address.ObjectID)
	}
	rel := instanceRelativePath(address)
	if dir := InstanceTypeDir(schemaRef); dir != "" {
		return dir + "/" + rel
	}
	return rel
}

func instanceRelativePath(address objectmodel.Address) string {
	if address.MemberKey != "" && address.AspectName != "" {
		return string(address.ObjectID) + "/" + address.AspectName + "/" + address.MemberKey + ".json"
	}
	if address.AspectName != "" {
		return string(address.ObjectID) + "/" + address.AspectName + ".json"
	}
	return string(address.ObjectID) + ".json"
}

// InstanceTypeDir is the Canonical type folder for an instance, derived from
// schema_ref (schema/metric.properties → metrics). Domain Schema stays in _schemas/.
func InstanceTypeDir(schemaRef string) string {
	parsed, ok := objectmodel.ParseSchemaRef(schemaRef)
	if !ok || !objectmodel.IsSchemaObject(parsed.Object) {
		return ""
	}
	rest := strings.TrimPrefix(string(parsed.Object), objectmodel.SchemaObjectPrefix)
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
	case "readme":
		return "readmes"
	}
	if strings.HasSuffix(entity, "s") {
		return entity
	}
	return entity + "s"
}

// DefaultSchemaPath is the Canonical tree path for a schema/* object.
// All Domain Schema files sit in one _schemas/ directory; identity remains object_id.
func DefaultSchemaPath(objectID objectmodel.ObjectID) string {
	rest := strings.TrimPrefix(string(objectID), objectmodel.SchemaObjectPrefix)
	if rest == "" || rest == string(objectID) {
		return objectmodel.CanonicalSchemaDir + "/" + string(objectID) + ".aspect.yaml"
	}
	parts := strings.Split(rest, "/")
	if n := len(parts); n >= 2 && isSchemaVersionSegment(parts[n-1]) {
		return objectmodel.CanonicalSchemaDir + "/" + parts[n-2] + "." + parts[n-1] + ".aspect.yaml"
	}
	return objectmodel.CanonicalSchemaDir + "/" + rest + ".aspect.yaml"
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
// schema/* always lands in the single _schemas/ directory. A readme aspect
// with no declared path lands at the repository root; an explicit path is kept.
func PathHintForIngest(address objectmodel.Address, declared, rel string) string {
	if objectmodel.IsSchemaObject(address.ObjectID) {
		return DefaultSchemaPath(address.ObjectID)
	}
	if address.Kind == objectmodel.KindAspect && address.AspectName == objectmodel.ReadmeAspect && address.MemberKey == "" {
		hint := strings.TrimSpace(declared)
		if hint == "" {
			hint = strings.TrimSpace(rel)
		}
		if hint == "" {
			return objectmodel.RepositoryReadmePath
		}
		return hint
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

func EntityPathHint(units []Unit, objectID objectmodel.ObjectID) string {
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
