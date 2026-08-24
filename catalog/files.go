package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
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

// Registry file names. Flat and one-per-record so `kc audit --workspace` can
// ask git for the history of a single Workspace, and a human can read the tree.
const (
	workspaceFilePrefix  = "workspace-"
	repositoryFilePrefix = "repository-"
	yamlExt              = ".yaml"
)

func CatalogFile() string { return "catalog" + yamlExt }

// WorkspaceYAML is the registry file for one Workspace recipe.
func WorkspaceYAML(workspaceID string) string {
	return workspaceFilePrefix + fileToken(workspaceID) + yamlExt
}
func RepositoryFile(repositoryID string) string {
	return repositoryFilePrefix + fileToken(repositoryID) + yamlExt
}

func encodeYAML(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var n any
	if err := json.Unmarshal(raw, &n); err != nil {
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
	var n any
	if err := yaml.Unmarshal(body, &n); err != nil {
		return err
	}
	raw, err := json.Marshal(n)
	if err != nil {
		return err
	}
	return DecodeJSON(raw, dest)
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
	case float64:
		if t == float64(int64(t)) {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(int64(t), 10)}
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(t, 'g', -1, 64)}
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
