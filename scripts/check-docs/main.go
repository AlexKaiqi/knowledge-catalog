// Command check-docs validates the git-tracked documentation graph.
// Nodes are Aspect OKF units; edges are Canonical Relation OKF units.
// Protocol shape is knowledge.DecodeRelation / repofile.Parse, not a parallel JSON schema.
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"kc/internal/repofile"
	"kc/knowledge"
)

const (
	documentationRepository = "kr://kc/documentation"
	documentPrefix          = "documentation/"
	catalogAspect           = "catalog-entry"
)

var (
	allowedClass = map[string]struct{}{
		"entrypoint": {}, "foundation": {}, "decision": {}, "runtime": {},
		"evolution": {}, "validation": {}, "guide": {},
	}
	allowedLifecycle = map[string]struct{}{
		"normative": {}, "active": {}, "draft": {}, "planned": {},
	}
	allowedRelation = map[string]struct{}{
		"depends_on": {}, "refines": {}, "verifies": {},
		"operationalizes": {}, "measures": {}, "catalogs": {},
	}
	mdLink                 = regexp.MustCompile(`\]\(([^)#]+)`)
	requiredDesignHeadings = []string{
		"## Goal",
		"## Non-Goals",
		"## 硬性约束 / Invariants",
		"## 选定方案 / 被否决方案",
		"## 接口契约 / 状态机",
	}
	designContractClass = map[string]struct{}{
		"foundation": {}, "decision": {}, "runtime": {}, "evolution": {},
	}
)

type catalogEntry struct {
	ID          string   `json:"id"`
	Path        string   `json:"path"`
	Title       string   `json:"title"`
	Class       string   `json:"class"`
	Lifecycle   string   `json:"lifecycle"`
	Audience    []string `json:"audience"`
	OwnerTopics []string `json:"ownerTopics"`
}

type edge struct {
	Type string
	From string
	To   string
}

func main() {
	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	docs, rels, err := loadGraph(filepath.Join(root, "docs", "graph"))
	if err != nil {
		fatal(err)
	}
	if err := validateCatalog(root, docs, rels); err != nil {
		fatal(err)
	}
	if err := validateDesignContracts(root, docs); err != nil {
		fatal(err)
	}
	if err := validateMarkdownLinks(root, docs); err != nil {
		fatal(err)
	}
	fmt.Printf("documentation graph: PASS (%d documents, %d relations, unique topics, valid links, acyclic depends_on, design contracts)\n",
		len(docs), len(rels))
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("go.mod not found from %s", wd)
		}
	}
}

func loadGraph(graphDir string) (map[string]catalogEntry, []edge, error) {
	docs := map[string]catalogEntry{}
	var rels []edge
	err := filepath.WalkDir(graphDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".okf") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		unit := repofile.Parse(string(raw))
		if unit == nil {
			return fmt.Errorf("%s: not a knowledge unit", relPath(graphDir, path))
		}
		rel := relPath(graphDir, path)
		switch unit.Address.Kind {
		case knowledge.KindAspect:
			entry, err := decodeDocument(*unit, rel)
			if err != nil {
				return err
			}
			if _, dup := docs[entry.ID]; dup {
				return fmt.Errorf("duplicate document id %s", entry.ID)
			}
			docs[entry.ID] = entry
		case knowledge.KindRelation:
			got, err := decodeEdge(*unit, rel)
			if err != nil {
				return err
			}
			rels = append(rels, got)
		default:
			return fmt.Errorf("%s: documentation graph only accepts Aspect catalog-entry or Relation units", rel)
		}
		return nil
	})
	return docs, rels, err
}

func decodeDocument(unit repofile.Unit, rel string) (catalogEntry, error) {
	if unit.Address.AspectName != catalogAspect {
		return catalogEntry{}, fmt.Errorf("%s: document Aspect must be %s", rel, catalogAspect)
	}
	want := knowledge.ObjectID(documentPrefix + strings.TrimPrefix(string(unit.ObjectID), documentPrefix))
	if unit.ObjectID != want || !strings.HasPrefix(string(unit.ObjectID), documentPrefix) {
		return catalogEntry{}, fmt.Errorf("%s: object_id must be documentation/<id>", rel)
	}
	body, err := json.Marshal(unit.Value)
	if err != nil {
		return catalogEntry{}, fmt.Errorf("%s: %w", rel, err)
	}
	var entry catalogEntry
	if err := json.Unmarshal(body, &entry); err != nil {
		return catalogEntry{}, fmt.Errorf("%s: catalog-entry JSON: %w", rel, err)
	}
	id := strings.TrimPrefix(string(unit.ObjectID), documentPrefix)
	if entry.ID != id {
		return catalogEntry{}, fmt.Errorf("%s: id %q must match object_id suffix %q", rel, entry.ID, id)
	}
	if entry.Path == "" || entry.Title == "" {
		return catalogEntry{}, fmt.Errorf("%s: path and title are required", rel)
	}
	if _, ok := allowedClass[entry.Class]; !ok {
		return catalogEntry{}, fmt.Errorf("%s: unknown class %q", rel, entry.Class)
	}
	if _, ok := allowedLifecycle[entry.Lifecycle]; !ok {
		return catalogEntry{}, fmt.Errorf("%s: unknown lifecycle %q", rel, entry.Lifecycle)
	}
	if len(entry.OwnerTopics) == 0 {
		return catalogEntry{}, fmt.Errorf("%s: ownerTopics must not be empty", rel)
	}
	return entry, nil
}

func decodeEdge(unit repofile.Unit, rel string) (edge, error) {
	decoded, err := knowledge.DecodeRelation(unit.Address, unit.Value)
	if err != nil {
		return edge{}, fmt.Errorf("%s: %w", rel, err)
	}
	if unit.SchemaRef != string(knowledge.CoreRelationSchemaV1) {
		return edge{}, fmt.Errorf("%s: schema_ref must be %s", rel, knowledge.CoreRelationSchemaV1)
	}
	if decoded.Direction != knowledge.RelationDirected {
		return edge{}, fmt.Errorf("%s: documentation relations are DIRECTED", rel)
	}
	if _, ok := allowedRelation[decoded.RelationType]; !ok {
		return edge{}, fmt.Errorf("%s: unknown relationType %q", rel, decoded.RelationType)
	}
	from, to := "", ""
	for _, endpoint := range decoded.Endpoints {
		if string(endpoint.ObjectRef.Repository) != documentationRepository {
			return edge{}, fmt.Errorf("%s: objectRef.repository must be %s", rel, documentationRepository)
		}
		if !strings.HasPrefix(string(endpoint.ObjectRef.Object), documentPrefix) {
			return edge{}, fmt.Errorf("%s: endpoint object must be documentation/<id>", rel)
		}
		id := strings.TrimPrefix(string(endpoint.ObjectRef.Object), documentPrefix)
		switch endpoint.Role {
		case "from":
			from = id
		case "to":
			to = id
		default:
			return edge{}, fmt.Errorf("%s: endpoint role must be from or to", rel)
		}
	}
	if from == "" || to == "" {
		return edge{}, fmt.Errorf("%s: relation needs from and to endpoints", rel)
	}
	return edge{Type: decoded.RelationType, From: from, To: to}, nil
}

func validateCatalog(root string, docs map[string]catalogEntry, rels []edge) error {
	if len(docs) == 0 {
		return fmt.Errorf("docs/graph has no document units")
	}
	paths := map[string]string{}
	topics := map[string]string{}
	for id, doc := range docs {
		if other, dup := paths[doc.Path]; dup {
			return fmt.Errorf("path %s owned by both %s and %s", doc.Path, other, id)
		}
		paths[doc.Path] = id
		abs := filepath.Join(root, filepath.FromSlash(doc.Path))
		if _, err := os.Stat(abs); err != nil {
			return fmt.Errorf("document does not exist: %s", doc.Path)
		}
		for _, topic := range doc.OwnerTopics {
			if other, dup := topics[topic]; dup {
				return fmt.Errorf("ownerTopic %q duplicated by %s and %s", topic, other, id)
			}
			topics[topic] = id
		}
	}
	actual, err := listedMarkdown(root)
	if err != nil {
		return err
	}
	var catalogPaths []string
	for path := range paths {
		catalogPaths = append(catalogPaths, path)
	}
	sort.Strings(catalogPaths)
	if strings.Join(actual, "\n") != strings.Join(catalogPaths, "\n") {
		return fmt.Errorf("top-level Markdown inventory differs from docs/graph/documents\nwant:\n%s\ngot:\n%s",
			strings.Join(catalogPaths, "\n"), strings.Join(actual, "\n"))
	}
	for _, rel := range rels {
		if _, ok := docs[rel.From]; !ok {
			return fmt.Errorf("relation from unknown document %s", rel.From)
		}
		if _, ok := docs[rel.To]; !ok {
			return fmt.Errorf("relation to unknown document %s", rel.To)
		}
	}
	return acyclicDepends(docs, rels)
}

func validateDesignContracts(root string, docs map[string]catalogEntry) error {
	var missing []string
	for _, doc := range docs {
		if _, ok := designContractClass[doc.Class]; !ok {
			continue
		}
		if !strings.HasSuffix(doc.Path, ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc.Path)))
		if err != nil {
			return err
		}
		body := string(raw)
		for _, heading := range requiredDesignHeadings {
			if !strings.Contains(body, heading+"\n") && !strings.Contains(body, heading+"\r\n") {
				missing = append(missing, fmt.Sprintf("%s missing %s", doc.Path, heading))
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("design docs must use ai-native-project-maintenance headings:\n  %s", strings.Join(missing, "\n  "))
	}
	return nil
}

func listedMarkdown(root string) ([]string, error) {
	var out []string
	out = append(out, "README.md")
	entries, err := os.ReadDir(filepath.Join(root, "docs"))
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		out = append(out, "docs/"+entry.Name())
	}
	sort.Strings(out)
	return out, nil
}

func acyclicDepends(docs map[string]catalogEntry, rels []edge) error {
	incoming := map[string]int{}
	outgoing := map[string][]string{}
	for id := range docs {
		incoming[id] = 0
	}
	for _, rel := range rels {
		if rel.Type != "depends_on" {
			continue
		}
		outgoing[rel.To] = append(outgoing[rel.To], rel.From)
		incoming[rel.From]++
	}
	var ready []string
	for id, n := range incoming {
		if n == 0 {
			ready = append(ready, id)
		}
	}
	seen := 0
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		seen++
		for _, next := range outgoing[id] {
			incoming[next]--
			if incoming[next] == 0 {
				ready = append(ready, next)
			}
		}
	}
	if seen != len(docs) {
		return fmt.Errorf("depends_on graph has a cycle")
	}
	return nil
}

func validateMarkdownLinks(root string, docs map[string]catalogEntry) error {
	var files []string
	for _, doc := range docs {
		if strings.HasSuffix(doc.Path, ".md") {
			files = append(files, doc.Path)
		}
	}
	sort.Strings(files)
	var broken []string
	for _, rel := range files {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		for _, match := range mdLink.FindAllStringSubmatch(string(raw), -1) {
			target := match[1]
			switch {
			case strings.HasPrefix(target, "http://"), strings.HasPrefix(target, "https://"), strings.HasPrefix(target, "mailto:"):
				continue
			}
			var resolved string
			if strings.HasPrefix(target, "/") {
				resolved = target
			} else {
				resolved = filepath.Join(root, filepath.Dir(filepath.FromSlash(rel)), filepath.FromSlash(target))
			}
			if _, err := os.Stat(resolved); err != nil {
				broken = append(broken, fmt.Sprintf("%s -> %s", rel, target))
			}
		}
	}
	if len(broken) > 0 {
		return fmt.Errorf("broken local Markdown link:\n  %s", strings.Join(broken, "\n  "))
	}
	return nil
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "%v\n", err)
	os.Exit(1)
}
