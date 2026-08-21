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

func CatalogFile() string           { return "catalog.yaml" }
func ViewFile(viewID string) string { return "view-" + fileToken(viewID) + ".yaml" }
func RepositoryFile(repositoryID string) string {
	return "repository-" + fileToken(repositoryID) + ".yaml"
}

func registryPath(objectID string) string {
	id := strings.TrimSpace(objectID)
	if id == "" {
		return ""
	}
	if strings.HasSuffix(id, ".yaml") {
		return id
	}
	switch {
	case id == "meta/catalog" || id == "catalog":
		return CatalogFile()
	case strings.HasPrefix(id, "view/"):
		return ViewFile(strings.TrimPrefix(id, "view/"))
	case strings.HasPrefix(id, "repository/"):
		return RepositoryFile(strings.TrimPrefix(id, "repository/"))
	case strings.HasPrefix(id, "member/"):
		return RepositoryFile(strings.TrimPrefix(id, "member/"))
	default:
		return fileToken(id) + ".yaml"
	}
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
	return json.Unmarshal(raw, dest)
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
