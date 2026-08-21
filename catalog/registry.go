package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kc/kernel"
	"kc/local"
)

// Registry persists ViewDefinition and registered repositories as
// flat YAML files in the registry git root (catalog.yaml, view-*.yaml, repository-*.yaml).
//
// It is not a knowledge Repository. Do not repo-add a Catalog id into a View.
// On disk each Catalog is <home>/catalogs/<encoded-id> (layout.catalogs).
// History of those files is catalog.Log.
type Registry struct {
	repo *local.FileGitRepository
}

func NewRegistry(rootDir string, repositoryID string) (*Registry, error) {
	repo, err := local.NewFileGit(rootDir, kernel.RepositoryID(repositoryID))
	if err != nil {
		return nil, err
	}
	return &Registry{repo: repo}, nil
}

func (g *Registry) Repo() *local.FileGitRepository { return g.repo }

func (g *Registry) CatalogID() string { return string(g.repo.ID()) }

func (g *Registry) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.repo.RootDir()
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func (g *Registry) headYAML() (map[string][]byte, error) {
	raw, err := g.git("ls-tree", "-r", "--name-only", "HEAD")
	if err != nil || raw == "" {
		return map[string][]byte{}, nil
	}
	out := map[string][]byte{}
	for _, path := range strings.Split(raw, "\n") {
		path = strings.TrimSpace(path)
		if path == "" || !strings.HasSuffix(path, ".yaml") || strings.Contains(path, "/") {
			continue
		}
		body, err := g.git("show", "HEAD:"+path)
		if err != nil {
			return nil, err
		}
		out[path] = []byte(body + "\n")
	}
	return out, nil
}

// Load reads registry YAML from HEAD. Old knowledge-object JSON trees still load until the next Save.
func (g *Registry) Load() (CatalogState, error) {
	files, err := g.headYAML()
	if err != nil {
		return CatalogState{}, err
	}
	if len(files) > 0 {
		return g.stateFromYAML(files)
	}
	return g.loadLegacyJSON()
}

func (g *Registry) stateFromYAML(files map[string][]byte) (CatalogState, error) {
	views := []ViewDefinition{}
	ids := []string{}
	archived := false
	catalogID := ""
	for path, body := range files {
		switch {
		case path == CatalogFile():
			var meta catalogMeta
			if err := decodeYAML(body, &meta); err != nil {
				return CatalogState{}, err
			}
			archived = meta.Archived
			catalogID = meta.ID
		case strings.HasPrefix(path, "view-"):
			def, err := asViewDefinitionYAML(body)
			if err != nil {
				return CatalogState{}, err
			}
			views = append(views, def)
		case strings.HasPrefix(path, "generation-"), strings.HasPrefix(path, "release-"):
			continue
		case strings.HasPrefix(path, "repository-"), strings.HasPrefix(path, "member-"):
			var row map[string]string
			if err := decodeYAML(body, &row); err != nil {
				return CatalogState{}, err
			}
			if id := row["repository"]; id != "" {
				ids = append(ids, id)
			} else if id := row["repositoryId"]; id != "" {
				ids = append(ids, id)
			}
		}
	}
	return NormalizeCatalogState(CatalogState{
		Views: views, Repositories: ids, Archived: archived,
		CatalogID: catalogID,
	}), nil
}

func (g *Registry) loadLegacyJSON() (CatalogState, error) {
	head, err := g.repo.Head("refs/heads/main")
	if err != nil {
		return EmptyCatalogState, err
	}
	listed, err := g.repo.List(head)
	if err != nil {
		return CatalogState{}, err
	}
	if len(listed) == 0 {
		return EmptyCatalogState, nil
	}
	views := []ViewDefinition{}
	ids := []string{}
	archived := false
	catalogID := ""
	for _, item := range listed {
		id := string(item.Address.ObjectID)
		switch {
		case strings.HasPrefix(id, "view/"):
			def, err := asViewDefinition(item.Value)
			if err != nil {
				return CatalogState{}, err
			}
			views = append(views, def)
		case strings.HasPrefix(id, "generation/"), strings.HasPrefix(id, "release/"):
			continue
		case strings.HasPrefix(id, "repository/"):
			ids = append(ids, strings.TrimPrefix(id, "repository/"))
		case strings.HasPrefix(id, "member/"):
			ids = append(ids, strings.TrimPrefix(id, "member/"))
		case id == "meta/catalog":
			meta, err := reJSON[catalogMeta](item.Value)
			if err != nil {
				return CatalogState{}, err
			}
			archived = meta.Archived
			catalogID = meta.ID
		}
	}
	return NormalizeCatalogState(CatalogState{
		Views: views, Repositories: ids, Archived: archived,
		CatalogID: catalogID,
	}), nil
}

// Save diffs CatalogState against HEAD and commits YAML files. Same digest is a no-op once YAML is the layout.
func (g *Registry) Save(state CatalogState, message, author, requestID, ruleID string) error {
	current, err := g.Load()
	if err != nil {
		return err
	}
	next := NormalizeCatalogState(state)
	files, err := g.headYAML()
	if err != nil {
		return err
	}
	if kernel.CanonicalDigest(current) == kernel.CanonicalDigest(next) && len(files) > 0 {
		return nil
	}
	if message == "" {
		message = "catalog: persist"
	}

	desired, err := yamlFiles(next, g.CatalogID())
	if err != nil {
		return err
	}
	head, err := g.repo.Head("refs/heads/main")
	if err != nil {
		return err
	}
	root := g.repo.RootDir()
	if _, err := g.git("checkout", "-q", "main"); err != nil {
		return err
	}
	if err := g.clearLegacyLayout(); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		if _, ok := desired[e.Name()]; !ok {
			if err := os.Remove(filepath.Join(root, e.Name())); err != nil {
				return err
			}
		}
	}
	for name, body := range desired {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o644); err != nil {
			return err
		}
	}
	_, err = g.repo.CommitWorktree(head, message, author, requestID, ruleID)
	return err
}

func yamlFiles(state CatalogState, catalogID string) (map[string][]byte, error) {
	out := map[string][]byte{}
	body, err := encodeYAML(catalogMeta{ID: catalogID, Archived: state.Archived})
	if err != nil {
		return nil, err
	}
	out[CatalogFile()] = body
	for _, view := range state.Views {
		b, err := encodeYAML(view)
		if err != nil {
			return nil, err
		}
		out[ViewFile(view.ViewID)] = b
	}
	for _, id := range state.Repositories {
		b, err := encodeYAML(map[string]string{"repository": id})
		if err != nil {
			return nil, err
		}
		out[RepositoryFile(id)] = b
	}
	return out, nil
}

func (g *Registry) clearLegacyLayout() error {
	root := g.repo.RootDir()
	for _, name := range []string{"meta", "view", "generation", "release", "member", "objects"} {
		if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
			return err
		}
	}
	return nil
}

type CatalogState struct {
	Views        []ViewDefinition `json:"views"`
	Repositories []string         `json:"repositories"`
	Archived     bool             `json:"archived,omitempty"`
	CatalogID    string           `json:"catalogId,omitempty"`
}

var EmptyCatalogState = CatalogState{
	Views:        []ViewDefinition{},
	Repositories: []string{},
}

func NormalizeCatalogState(state CatalogState) CatalogState {
	views := append([]ViewDefinition{}, state.Views...)
	sortViews(views)
	ids := append([]string{}, state.Repositories...)
	sortStrings(ids)
	ids = uniqueStrings(ids)
	return CatalogState{
		Views: views, Repositories: ids, Archived: state.Archived,
		CatalogID: state.CatalogID,
	}
}

func sortStrings(items []string) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j] < items[i] {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

func uniqueStrings(items []string) []string {
	out := items[:0]
	var prev string
	for i, s := range items {
		if i == 0 || s != prev {
			out = append(out, s)
			prev = s
		}
	}
	return out
}

func sortViews(views []ViewDefinition) {
	for i := 0; i < len(views); i++ {
		for j := i + 1; j < len(views); j++ {
			if views[j].ViewID < views[i].ViewID {
				views[i], views[j] = views[j], views[i]
			}
		}
	}
}

// CatalogsPath is the parent directory of Catalog registry gits (not a View source).
func CatalogsPath(home string) string {
	return filepath.Join(home, "catalogs")
}

// DefaultCatalogID is the first Catalog when `kc init` omits --catalog.
const DefaultCatalogID = "kr://local/catalog"

// PeekID reads catalog.yaml id from HEAD. The directory is a registry git, not a knowledge repository.
func PeekID(rootDir string) (string, error) {
	cmd := exec.Command("git", "show", "HEAD:"+CatalogFile())
	cmd.Dir = rootDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var meta catalogMeta
	if err := decodeYAML(out, &meta); err != nil {
		return "", err
	}
	if meta.ID == "" {
		return "", fmt.Errorf("catalog.yaml missing id in %s", rootDir)
	}
	return meta.ID, nil
}

func asViewDefinition(value any) (ViewDefinition, error) {
	return reJSON[ViewDefinition](value)
}

func asViewDefinitionYAML(body []byte) (ViewDefinition, error) {
	var def ViewDefinition
	err := decodeYAML(body, &def)
	return def, err
}

func reJSON[T any](value any) (T, error) {
	var zero T
	b, err := json.Marshal(value)
	if err != nil {
		return zero, err
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, err
	}
	return out, nil
}
