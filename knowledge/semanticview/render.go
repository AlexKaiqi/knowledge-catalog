// Package semanticview renders immutable Knowledge values as human- and
// Agent-readable YAML files. The output is a disposable consumer projection:
// it preserves canonical coordinates but is never a Writer input or authority.
package semanticview

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"kc/kernel"
	"kc/knowledge"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Metadata keeps a rendered file traceable to the exact canonical value.
type Metadata struct {
	ObjectID   knowledge.ObjectID  `yaml:"object_id"`
	Repository kernel.RepositoryID `yaml:"repository"`
	Commit     kernel.CommitID     `yaml:"commit"`
	Schemas    map[string]string   `yaml:"schemas,omitempty"`
}

type document struct {
	Metadata Metadata       `yaml:"_kc"`
	Fields   map[string]any `yaml:",inline,omitempty"`
	Value    any            `yaml:"value,omitempty"`
}

// Render emits actual assembled YAML values, not the canonical storage
// envelope. _kc is sufficient to return to structured READ/PROVENANCE.
func Render(value knowledge.KnowledgeValue) ([]byte, error) {
	schemas := map[string]string{}
	declarations := append([]knowledge.UnitDeclaration(nil), value.Declarations...)
	sort.Slice(declarations, func(i, j int) bool {
		return knowledge.AddressKey(declarations[i].Address) < knowledge.AddressKey(declarations[j].Address)
	})
	for _, declaration := range declarations {
		if strings.TrimSpace(declaration.SchemaRef) == "" {
			continue
		}
		key := "entity"
		if declaration.Address.AspectName != "" {
			key = declaration.Address.AspectName
		}
		if declaration.Address.MemberKey != "" {
			key += "/" + declaration.Address.MemberKey
		}
		schemas[key] = declaration.SchemaRef
	}
	doc := document{Metadata: Metadata{
		ObjectID: value.Address.ObjectID, Repository: value.Repository, Commit: value.Commit,
		Schemas: schemas,
	}}
	if object, ok := value.Value.(map[string]any); ok {
		doc.Fields = object
	} else {
		doc.Value = value.Value
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("render semantic YAML for %s: %w", value.Address.ObjectID, err)
	}
	return out, nil
}

// Path chooses a readable, deterministic projection path. Entity comes from
// the exact Domain Schema resolved at the same Repository commit.
func Path(value knowledge.KnowledgeValue, entity string) string {
	if knowledge.IsSchemaObject(value.Address.ObjectID) {
		return "schemas/" + Slug(string(value.Address.ObjectID)) + ".yaml"
	}
	if relation(value) {
		return "relations/" + fileStem(value) + ".yaml"
	}
	directory := plural(Slug(entity))
	if directory == "" {
		directory = "objects"
	}
	return directory + "/" + fileStem(value) + ".yaml"
}

func relation(value knowledge.KnowledgeValue) bool {
	if value.Address.Kind == knowledge.KindRelation {
		return true
	}
	for _, unit := range value.Units {
		if unit.Kind == knowledge.KindRelation {
			return true
		}
	}
	return false
}

func fileStem(value knowledge.KnowledgeValue) string {
	name := displayName(value.Value)
	if name == "" {
		name = string(value.Address.ObjectID)
	}
	base := Slug(name)
	if base == "" {
		base = "knowledge"
	}
	digest := string(kernel.CanonicalDigest(string(value.Address.ObjectID)))
	if len(digest) > 8 {
		digest = digest[:8]
	}
	return base + "--" + digest
}

func displayName(value any) string {
	body, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if name, ok := body["name"].(string); ok {
		return name
	}
	for _, section := range []string{"definition", "properties"} {
		if child, ok := body[section].(map[string]any); ok {
			if name, ok := child["name"].(string); ok {
				return name
			}
		}
	}
	return ""
}

// Slug turns protocol names and labels into portable lower-kebab paths.
func Slug(value string) string {
	var expanded strings.Builder
	for i, r := range strings.TrimSpace(value) {
		if i > 0 && unicode.IsUpper(r) {
			expanded.WriteByte('-')
		}
		expanded.WriteRune(unicode.ToLower(r))
	}
	return strings.Trim(nonSlug.ReplaceAllString(expanded.String(), "-"), "-")
}

func plural(value string) string {
	if value == "" || strings.HasSuffix(value, "s") {
		return value
	}
	if strings.HasSuffix(value, "y") && len(value) > 1 {
		return strings.TrimSuffix(value, "y") + "ies"
	}
	if strings.HasSuffix(value, "ch") || strings.HasSuffix(value, "sh") || strings.HasSuffix(value, "x") {
		return value + "es"
	}
	return value + "s"
}
