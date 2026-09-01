package cli

import (
	"bytes"
	"path"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"kc/catalog"
	"kc/kernel"
	"kc/knowledge"
	knowledgemaintenance "kc/knowledge/maintenance"
	"kc/knowledge/semanticview"
	"kc/snapshot"
)

const semanticFileViewV1 = "semantic-yaml/v1"

type semanticProjection struct {
	files       map[string][]byte
	directories map[string][]snapshot.DirectoryEntry
}

type semanticProjectionKey struct {
	repository kernel.RepositoryID
	commit     kernel.CommitID
}

var semanticProjections = struct {
	sync.Mutex
	entries map[semanticProjectionKey]*semanticProjection
	order   []semanticProjectionKey
}{entries: map[semanticProjectionKey]*semanticProjection{}}

func semanticMounts(def catalog.WorkspaceDefinition, pin catalog.ResolvedWorkspace) []catalog.VirtualMount {
	selectors := map[kernel.RepositoryID]string{}
	for _, source := range def.Sources {
		if _, exists := selectors[source.Repository]; !exists {
			selectors[source.Repository] = source.Selector
		}
	}
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for repository := range pin.Repositories {
		ids = append(ids, repository)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	used := map[string]bool{}
	mounts := make([]catalog.VirtualMount, 0, len(ids))
	for _, repository := range ids {
		label := semanticview.Slug(path.Base(string(repository)))
		if label == "" {
			label = "repository"
		}
		mountPath := "knowledge/" + label
		if used[mountPath] {
			digest := string(kernel.CanonicalDigest(string(repository)))
			mountPath += "-" + digest[:8]
		}
		used[mountPath] = true
		mounts = append(mounts, catalog.VirtualMount{
			Path: mountPath, Repository: repository, Commit: pin.Repositories[repository],
			Selector: selectors[repository], SubPath: semanticFileViewV1,
		})
	}
	return mounts
}

func semanticProjectionFor(repo knowledge.Repository, commit kernel.CommitID) (*semanticProjection, error) {
	key := semanticProjectionKey{repository: repo.ID(), commit: commit}
	semanticProjections.Lock()
	defer semanticProjections.Unlock()
	if cached := semanticProjections.entries[key]; cached != nil {
		return cached, nil
	}
	projection, err := buildSemanticProjection(repo, commit)
	if err != nil {
		return nil, err
	}
	semanticProjections.entries[key] = projection
	semanticProjections.order = append(semanticProjections.order, key)
	if len(semanticProjections.order) > 32 {
		oldest := semanticProjections.order[0]
		semanticProjections.order = semanticProjections.order[1:]
		delete(semanticProjections.entries, oldest)
	}
	return projection, nil
}

func buildSemanticProjection(repo knowledge.Repository, commit kernel.CommitID) (*semanticProjection, error) {
	scanner, err := knowledgemaintenance.RequireScanner(repo)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s cannot build %s: %v", repo.ID(), semanticFileViewV1, err)
	}
	values := []knowledge.KnowledgeValue{}
	if err := knowledgemaintenance.WalkSnapshot(scanner, commit, func(value knowledge.KnowledgeValue) error {
		values = append(values, value)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Address.ObjectID < values[j].Address.ObjectID })
	schemas := map[knowledge.ObjectID]knowledge.SchemaDefinition{}
	for _, value := range values {
		if !knowledge.IsSchemaObject(value.Address.ObjectID) {
			continue
		}
		definition, parseErr := knowledge.ParseSchemaDefinition(value.Address.ObjectID, value.Value)
		if parseErr != nil {
			return nil, parseErr
		}
		schemas[value.Address.ObjectID] = definition
	}
	projection := &semanticProjection{files: map[string][]byte{}, directories: map[string][]snapshot.DirectoryEntry{}}
	meta, _ := yaml.Marshal(map[string]any{
		"view": semanticFileViewV1, "repository": repo.ID(), "commit": commit,
		"canonical": false, "readOnly": true,
	})
	projection.files["_meta/repository.yaml"] = meta
	for _, value := range values {
		entity := semanticEntity(value, schemas)
		file := semanticview.Path(value, entity)
		raw, renderErr := semanticview.Render(value)
		if renderErr != nil {
			return nil, renderErr
		}
		if _, exists := projection.files[file]; exists {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "semantic projection path collision at %s", file)
		}
		projection.files[file] = raw
	}
	projection.indexDirectories()
	return projection, nil
}

func semanticEntity(value knowledge.KnowledgeValue, schemas map[knowledge.ObjectID]knowledge.SchemaDefinition) string {
	for _, declaration := range value.Declarations {
		parsed, ok := knowledge.ParseSchemaRef(declaration.SchemaRef)
		if !ok {
			continue
		}
		if schema, exists := schemas[parsed.Object]; exists {
			return schema.Entity
		}
	}
	return ""
}

func (p *semanticProjection) indexDirectories() {
	children := map[string]map[string]string{}
	for file := range p.files {
		parts := strings.Split(file, "/")
		for i, name := range parts {
			directory := strings.Join(parts[:i], "/")
			kind := "directory"
			if i == len(parts)-1 {
				kind = "file"
			}
			if children[directory] == nil {
				children[directory] = map[string]string{}
			}
			children[directory][name] = kind
		}
	}
	for directory, byName := range children {
		entries := make([]snapshot.DirectoryEntry, 0, len(byName))
		for name, kind := range byName {
			entries = append(entries, snapshot.DirectoryEntry{Name: name, Kind: kind})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		p.directories[directory] = entries
	}
}

func (p *semanticProjection) list(directory string, limit int, continuation string) ([]snapshot.DirectoryEntry, string, bool, error) {
	directory = strings.Trim(path.Clean("/"+directory), "/")
	if limit < 0 {
		return nil, "", false, kernel.Fail(kernel.ErrUsageInvalid, "directory limit cannot be negative")
	}
	if limit == 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}
	entries := p.directories[directory]
	start := 0
	if continuation != "" {
		start = sort.Search(len(entries), func(i int) bool { return entries[i].Name > continuation })
	}
	end := start + limit
	if end > len(entries) {
		end = len(entries)
	}
	next := ""
	if end < len(entries) && end > start {
		next = entries[end-1].Name
	}
	return append([]snapshot.DirectoryEntry(nil), entries[start:end]...), next, end == len(entries), nil
}

func (p *semanticProjection) read(file string) ([]byte, error) {
	file = strings.Trim(path.Clean("/"+file), "/")
	content, ok := p.files[file]
	if !ok {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "semantic file %s does not exist", file)
	}
	return bytes.Clone(content), nil
}
