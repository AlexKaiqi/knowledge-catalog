package dataset

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var fileTokenRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func fileToken(s string) string {
	t := fileTokenRe.ReplaceAllString(strings.TrimSpace(s), "_")
	t = strings.Trim(t, "._-")
	if t == "" {
		t = "x"
	}
	return t
}

// registry metadata includes the membership list. Dataset files remain separate
// so audit can filter by Dataset; releases provide immutable version addressing.
const (
	datasetFilePrefix    = "dataset-"
	datasetVersionPrefix = "release-"
	repositoryFilePrefix = "repository-"
	yamlExt              = ".yaml"
)

func isDatasetRegistryFile(path string) bool {
	return strings.HasPrefix(path, datasetFilePrefix)
}

func RegistryFile() string { return "registry" + yamlExt }

func datasetVersionFile(setID string, revision int) string {
	return fmt.Sprintf("%s%x-v%d%s", datasetVersionPrefix, sha256.Sum256([]byte(setID)), revision, yamlExt)
}

// KnowledgeSetYAML is the registry file for one Dataset recipe.
func KnowledgeSetYAML(setID string) string {
	return datasetFilePrefix + fileToken(setID) + yamlExt
}

// RepositoryFile identifies a read-compatible old member record. New saves
// put all members in registry.yaml instead of creating this file.
func RepositoryFile(repositoryID string) string {
	return repositoryFilePrefix + fileToken(repositoryID) + yamlExt
}

func encodeYAML(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var n any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&n); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{yamlNode(n)}}
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeYAML(body []byte, dest any) error {
	dec := yaml.NewDecoder(bytes.NewReader(body))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("registry YAML must be an object")
	}
	if err := validateRegistryYAMLNode(&doc); err != nil {
		return err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("registry YAML must contain exactly one document")
	}
	var n any
	if err := doc.Decode(&n); err != nil {
		return err
	}
	raw, err := json.Marshal(n)
	if err != nil {
		return err
	}
	return DecodeJSON(raw, dest)
}

func validateRegistryYAMLNode(node *yaml.Node) error {
	if node.Tag == "!!null" {
		return fmt.Errorf("registry YAML does not accept null fields")
	}
	if node.Tag == "!!float" {
		return fmt.Errorf("registry YAML numbers must be integers")
	}
	if node.Kind == yaml.ScalarNode && node.Tag != "!!str" && node.Tag != "!!int" && node.Tag != "!!bool" {
		return fmt.Errorf("registry YAML does not accept scalar type %s", node.Tag)
	}
	for _, child := range node.Content {
		if err := validateRegistryYAMLNode(child); err != nil {
			return err
		}
	}
	return nil
}

// DecodeJSON decodes the current Workspace vocabulary only.
func DecodeJSON(body []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	return dec.Decode(dest)
}

func yamlNode(n any) *yaml.Node {
	switch t := n.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	case bool:
		v := "false"
		if t {
			v = "true"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: v}
	case json.Number:
		tag := "!!int"
		if _, err := t.Int64(); err != nil {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: t.String()}
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: t}
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range t {
			node.Content = append(node.Content, yamlNode(item))
		}
		return node
	case map[string]any:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			node.Content = append(node.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k},
				yamlNode(t[k]),
			)
		}
		return node
	default:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fmt.Sprint(t)}
	}
}
