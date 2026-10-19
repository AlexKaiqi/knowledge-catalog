package invocation

// The dataset access layer: registry selection, workspace recipes, dataset
// resolution and replay pins, semantic views, and the file-read authorization
// that both the argv consume path and the HTTP file service run through.
// (CAP-ORG-01 B6: shared execution moves into the seam.)

import (
	"bytes"
	"os"
	"path"

	"gopkg.in/yaml.v3"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	kchome "kc/home"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	objectmaintenance "kc/object/maintenance"
	reader "kc/object/reader"
	"kc/object/semanticview"
	"kc/snapshot"
	"kc/snapshot/dataset"
	fileservice "kc/snapshot/service"
)

type SemanticProjection struct {
	files       map[string][]byte
	directories map[string][]snapshot.DirectoryEntry
}

// SemanticFileViewV1 is the semantic view coordinate scheme.
const SemanticFileViewV1 = "semantic-yaml/v1"

type semanticProjectionKey struct {
	repository kernel.RepositoryID
	commit     kernel.CommitID
	scope      kernel.Digest
}

// semanticProjections caches projections of immutable commits. The mutex
// guards only the map; a build runs outside it, and concurrent requests for
// the same key wait for that one build instead of every request waiting.
var semanticProjections = newSemanticProjectionCache(32)

type semanticProjectionCache struct {
	mu      sync.Mutex
	limit   int
	entries map[semanticProjectionKey]*semanticProjectionBuild
	order   []semanticProjectionKey
}

type semanticProjectionBuild struct {
	done       chan struct{}
	projection *SemanticProjection
	err        error
}

func newSemanticProjectionCache(limit int) *semanticProjectionCache {
	return &semanticProjectionCache{limit: limit, entries: map[semanticProjectionKey]*semanticProjectionBuild{}}
}

func (c *semanticProjectionCache) get(key semanticProjectionKey, build func() (*SemanticProjection, error)) (*SemanticProjection, error) {
	c.mu.Lock()
	if pending := c.entries[key]; pending != nil {
		c.mu.Unlock()
		<-pending.done
		return pending.projection, pending.err
	}
	pending := &semanticProjectionBuild{done: make(chan struct{})}
	c.entries[key] = pending
	c.order = append(c.order, key)
	if len(c.order) > c.limit {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
	c.mu.Unlock()

	pending.projection, pending.err = build()
	close(pending.done)
	if pending.err != nil {
		c.forget(key, pending)
	}
	return pending.projection, pending.err
}

// forget drops a failed build so a later request retries; waiters already
// holding it still receive its error.
func (c *semanticProjectionCache) forget(key semanticProjectionKey, pending *semanticProjectionBuild) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries[key] != pending {
		return
	}
	delete(c.entries, key)
	c.order = slices.DeleteFunc(c.order, func(k semanticProjectionKey) bool { return k == key })
}

func (p *SemanticProjection) indexDirectories() {
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

func PickDatasets(ws *kchome.Home, _ map[string]invoke.FlagValue) (dataset.Service, error) {
	if ws == nil || ws.Datasets == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Dataset service is unavailable")
	}
	return ws.Datasets, nil
}

// EnsureWorkspace returns the Registry recipe for setID, adopting a hitchhiking
// .kc-dataset.yaml from an attached member (or a local unanchored copy) when
// this machine has never defined it. Registry remains the operational store;
// the yaml is how the recipe travels with git (doc/design/dataset.md).
func EnsureWorkspace(ws *kchome.Home, home string, cat dataset.Service, setID string) (dataset.Dataset, error) {
	def, orig := cat.Set(setID)
	if ws.Deployment != nil {
		return def, orig
	}
	if orig == nil {
		return def, nil
	}
	rec, err := FindDatasetRecipe(ws, home, setID)
	if err != nil {
		return dataset.Dataset{}, err
	}
	if rec.Name == "" {
		return dataset.Dataset{}, orig
	}
	prepared, err := cat.PrepareDataset(setID, 1, rec.Sources())
	if err != nil {
		return dataset.Dataset{}, err
	}
	prepared.Metadata = rec.Metadata
	return cat.PublishDataset(prepared)
}

func FindDatasetRecipe(ws *kchome.Home, home, setID string) (dataset.DatasetRecipe, error) {
	if raw, err := os.ReadFile(LocalWorkspaceFile(home, setID)); err == nil {
		rec, err := dataset.ParseDatasetRecipe(raw)
		if err != nil {
			return dataset.DatasetRecipe{}, err
		}
		if rec.Name == setID {
			return rec, nil
		}
	}
	var hits []dataset.DatasetRecipe
	var from []kernel.RepositoryID
	for _, id := range ws.Store.IDs() {
		rec, ok := ReadRecipeAtHead(ws, id)
		if !ok || rec.Name != setID {
			continue
		}
		hits = append(hits, rec)
		from = append(from, id)
	}
	if len(hits) == 0 {
		return dataset.DatasetRecipe{}, nil
	}
	for i := 1; i < len(hits); i++ {
		a, _ := dataset.FormatDatasetRecipe(hits[0])
		b, _ := dataset.FormatDatasetRecipe(hits[i])
		if !bytes.Equal(a, b) {
			return dataset.DatasetRecipe{}, kernel.Fail(kernel.ErrDatasetInvalid,
				"workspace %s is declared differently in %s and %s", setID, from[0], from[i])
		}
	}
	return hits[0], nil
}

func LocalWorkspaceFile(home, name string) string {
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return filepath.Join(home, "datasets", safe+".yaml")
}

func ReadRecipeAtHead(ws *kchome.Home, id kernel.RepositoryID) (dataset.DatasetRecipe, bool) {
	snap, ok := ws.Store.Get(id)
	if !ok {
		return dataset.DatasetRecipe{}, false
	}
	files, ok := snapshot.TreeStoreOf(snap)
	if !ok {
		return dataset.DatasetRecipe{}, false
	}
	commit, err := snap.Head(snapshot.DefaultRef)
	if err != nil {
		return dataset.DatasetRecipe{}, false
	}
	raw, err := files.ReadFile(dataset.DatasetFileName, commit)
	if err != nil {
		return dataset.DatasetRecipe{}, false
	}
	rec, err := dataset.ParseDatasetRecipe(raw)
	if err != nil {
		return dataset.DatasetRecipe{}, false
	}
	return rec, true
}

func ApplyOverlay(home, as string, def dataset.Dataset) (dataset.Dataset, error) {
	raw, err := os.ReadFile(dataset.OverlayFile(home, as, def.SetID))
	if err != nil {
		if os.IsNotExist(err) {
			return def, nil
		}
		return dataset.Dataset{}, err
	}
	over, err := dataset.ParseDatasetOverlay(raw)
	if err != nil {
		return dataset.Dataset{}, err
	}
	return dataset.MergeOverlay(def, over)
}

func ResolveOrReplay(ws *kchome.Home, home string, cat dataset.Service, setID string, flags map[string]invoke.FlagValue) (dataset.ResolvedDataset, error) {
	return ResolveDataset(ws, home, cat, setID, flags)
}

func ResolveDataset(ws *kchome.Home, home string, cat dataset.Service, setID string, flags map[string]invoke.FlagValue) (dataset.ResolvedDataset, error) {
	var def dataset.Dataset
	if supplied := SuppliedDataset(flags); supplied != nil {
		// Temporary definitions replay through the pin; a caller label must not
		// borrow a published Workspace grant when an explicit recipe is present.
		setID = ""
		def = *supplied
		if err := dataset.ValidateDataset(def); err != nil {
			return dataset.ResolvedDataset{}, err
		}
		for _, source := range def.Sources {
			if !cat.HasRepository(source.Repository) {
				return dataset.ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "repository %s is not registered in this registry", source.Repository)
			}
		}
	} else {
		var err error
		def, err = EnsureWorkspace(ws, home, cat, setID)
		// The local unrestricted owner may inspect a personal overlay. It is
		// never used for a server identity's named Dataset grant or pin replay.
		if err == nil && ws.Deployment == nil && OwnerBypass(flags) && invoke.FlagString(flags, "pin") == "" {
			def, err = ApplyOverlay(home, "", def)
		}
		if err != nil {
			return dataset.ResolvedDataset{}, err
		}
	}
	if pinPath := invoke.FlagString(flags, "pin"); pinPath != "" {
		var resolved dataset.ResolvedDataset
		pin, replayErr := ParseReplayPin(pinPath)
		if replayErr == nil {
			if SuppliedDataset(flags) == nil {
				if pin.SetID != "" && pin.SetID != setID {
					return dataset.ResolvedDataset{}, kernel.Fail(kernel.ErrUsageInvalid, "pin names a different dataset")
				}
				pin.SetID = setID
				resolved, replayErr = cat.ReplayPublished(pin)
			} else {
				resolved, replayErr = ReplayPin(cat, def, pinPath)
			}
		}
		if replayErr == nil {
			flags[ResolvedPinFlag] = resolved.PinID
		}
		return resolved, replayErr
	}
	resolved, resolveErr := cat.ResolveDefinition(def)
	if resolveErr == nil {
		flags[ResolvedPinFlag] = resolved.PinID
	}
	return resolved, resolveErr
}

func ParseReplayPin(pinPath string) (dataset.ResolvedDataset, error) {
	raw := []byte(pinPath)
	if !strings.HasPrefix(strings.TrimSpace(pinPath), "{") {
		var err error
		raw, err = os.ReadFile(pinPath)
		if err != nil {
			return dataset.ResolvedDataset{}, err
		}
	}
	var pin dataset.ResolvedDataset
	if err := dataset.DecodeJSON(raw, &pin); err != nil {
		return dataset.ResolvedDataset{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin is not a ResolvedDataset JSON file")
	}
	return pin, nil
}

func ReplayPin(cat dataset.Service, def dataset.Dataset, pinPath string) (dataset.ResolvedDataset, error) {
	pin, err := DecodeReplayPin(def, pinPath)
	if err != nil {
		return dataset.ResolvedDataset{}, err
	}
	if check := cat.CheckResolved(pin); check.Outcome != "PASSED" {
		return dataset.ResolvedDataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "replayed pin failed CheckResolved")
	}
	return pin, nil
}

func DecodeReplayPin(def dataset.Dataset, pinPath string) (dataset.ResolvedDataset, error) {
	pin, err := ParseReplayPin(pinPath)
	if err != nil {
		return dataset.ResolvedDataset{}, err
	}
	return dataset.ReplayDefinition(def, pin)
}

func AllowedDatasetFileRead(home string, flags map[string]invoke.FlagValue) bool {
	return allowedDatasetFileRead(home, flags, "")
}

func allowedDatasetFileRead(home string, flags map[string]invoke.FlagValue, repository string) bool {
	as := invoke.FlagString(flags, "as")
	if as == "" {
		return true
	}
	file, err := ReadAllow(home)
	if err != nil {
		return false
	}
	q := AllowQuery{
		Principal: as,
		Action:    "file.read",
		Repo:      repository,
		Dataset:   invoke.DatasetOf(flags),
	}
	_, ok := MatchAllow(withDatasetDefaults(home, as, file.Rules), q)
	return ok
}

func SemanticMounts(def dataset.Dataset, pin dataset.ResolvedDataset) []dataset.VirtualMount {
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
	mounts := make([]dataset.VirtualMount, 0, len(ids))
	for _, repository := range ids {
		label := semanticview.Slug(path.Base(string(repository)))
		if label == "" {
			label = "repository"
		}
		mountPath := "object/" + label
		if used[mountPath] {
			digest := string(kernel.CanonicalDigest(string(repository)))
			mountPath += "-" + digest[:8]
		}
		used[mountPath] = true
		mounts = append(mounts, dataset.VirtualMount{
			Path: mountPath, Repository: repository, Commit: pin.Repositories[repository],
			Selector: selectors[repository], SubPath: SemanticFileViewV1,
		})
	}
	return mounts
}

func SemanticProjectionFor(store snapshot.Store, repo objectmodel.Repository, pin dataset.ResolvedDataset) (*SemanticProjection, error) {
	commit := pin.Repositories[repo.ID()]
	key := semanticProjectionKey{repository: repo.ID(), commit: commit, scope: kernel.CanonicalDigest(pin.Items)}
	return semanticProjections.get(key, func() (*SemanticProjection, error) {
		return BuildSemanticProjection(store, repo, pin)
	})
}

func BuildSemanticProjection(store snapshot.Store, repo objectmodel.Repository, pin dataset.ResolvedDataset) (*SemanticProjection, error) {
	commit := pin.Repositories[repo.ID()]
	scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, kchome.DatasetPin(pin))
	values := []objectmodel.ObjectValue{}
	var prefixes, files []string
	for _, item := range pin.Items {
		if item.Repository != repo.ID() || item.Commit != commit {
			continue
		}
		if item.Kind == dataset.DatasetItemPrefix {
			prefixes = append(prefixes, item.Prefix)
		}
		if item.Kind == dataset.DatasetItemFile {
			files = append(files, item.File)
		}
	}
	if err := objectmaintenance.WalkScope(store, repo, commit, prefixes, files, func(id objectmodel.ObjectID) error {
		allowed, err := scope.Contains(repo.ID(), id)
		if err != nil || !allowed {
			return err
		}
		value, err := repo.Read(id, commit)
		if err != nil {
			return err
		}
		values = append(values, value)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Address.ObjectID < values[j].Address.ObjectID })
	schemas := map[objectmodel.ObjectID]objectmodel.SchemaDefinition{}
	for _, value := range values {
		if !objectmodel.IsSchemaObject(value.Address.ObjectID) {
			continue
		}
		definition, parseErr := objectmodel.ParseSchemaDefinition(value.Address.ObjectID, value.Value)
		if parseErr != nil {
			return nil, parseErr
		}
		schemas[value.Address.ObjectID] = definition
	}
	projection := &SemanticProjection{files: map[string][]byte{}, directories: map[string][]snapshot.DirectoryEntry{}}
	meta, _ := yaml.Marshal(map[string]any{
		"view": SemanticFileViewV1, "repository": repo.ID(), "commit": commit,
		"canonical": false, "readOnly": true,
	})
	projection.files["_meta/repository.yaml"] = meta
	for _, value := range values {
		entity := SemanticEntity(value, schemas)
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

func SemanticEntity(value objectmodel.ObjectValue, schemas map[objectmodel.ObjectID]objectmodel.SchemaDefinition) string {
	for _, declaration := range value.Declarations {
		parsed, ok := objectmodel.ParseSchemaRef(declaration.SchemaRef)
		if !ok {
			continue
		}
		if schema, exists := schemas[parsed.Object]; exists {
			return schema.Entity
		}
	}
	return ""
}

func DatasetFSMayReadRepository(home string, flags map[string]invoke.FlagValue, repository string, opened ...*kchome.Home) (bool, error) {
	if OwnerBypass(flags) {
		return true, nil
	}
	// File consumption has a fixed source version, but no application-level
	// Object or Aspect coordinate. Keep authorization errors distinguishable
	// from a caller's lack of permission so inventory cannot hide a failure.
	query := map[string]invoke.FlagValue{
		"as": invoke.FlagString(flags, "as"), "dataset": invoke.DatasetOf(flags),
		"repo": repository, "ref": invoke.FlagString(flags, "ref"),
	}
	err := Authorize(home, "file.read", query, nil, opened...)
	if kernel.CodeOf(err) == kernel.ErrForbidden {
		return false, nil
	}
	return err == nil, err
}

func (p *SemanticProjection) List(directory string, limit int, continuation string) ([]snapshot.DirectoryEntry, string, bool, error) {
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
	return append([]snapshot.DirectoryEntry{}, entries[start:end]...), next, end == len(entries), nil
}
func (p *SemanticProjection) Read(file string) ([]byte, error) {
	file = strings.Trim(path.Clean("/"+file), "/")
	content, ok := p.files[file]
	if !ok {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "semantic file %s does not exist", file)
	}
	return bytes.Clone(content), nil
}

func DatasetFSRepositoryPath(subPath, relative string) (string, error) {
	return fileservice.RepositoryPath(subPath, relative)
}
func ValidDatasetFSRelative(value string) bool { return fileservice.ValidRelativePath(value) }

func RejectMixedObjectBasis(flags map[string]invoke.FlagValue) error {
	if invoke.FlagString(flags, "dataset") == "" && SuppliedDataset(flags) == nil && invoke.FlagString(flags, "pin") == "" && invoke.FlagString(flags, "dataset-file") == "" {
		return nil
	}
	if invoke.FlagString(flags, "repo") != "" || invoke.FlagString(flags, "commit") != "" || invoke.FlagString(flags, "ref") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "choose a Workspace definition/pin or a Repository basis; do not mix")
	}
	return nil
}
