package invocation

// The dataset access layer: registry selection, workspace recipes, knowledge-set
// resolution and replay pins, semantic views, and the file-read authorization
// that both the argv consume path and the HTTP file service run through.
// (CAP-ORG-01 B6: shared execution moves into the seam.)

import (
	"bytes"
	"os"
	"path"

	"gopkg.in/yaml.v3"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	kchome "kc/home"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	knowledgemaintenance "kc/object/maintenance"
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

var semanticProjections = struct {
	sync.Mutex
	entries map[semanticProjectionKey]*SemanticProjection
	order   []semanticProjectionKey
}{entries: map[semanticProjectionKey]*SemanticProjection{}}

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
func EnsureWorkspace(ws *kchome.Home, home string, cat dataset.Service, setID string) (dataset.KnowledgeSet, error) {
	def, orig := cat.Set(setID)
	if ws.Deployment != nil {
		return def, orig
	}
	if orig == nil {
		return def, nil
	}
	rec, err := FindKnowledgeSetRecipe(ws, home, setID)
	if err != nil {
		return dataset.KnowledgeSet{}, err
	}
	if rec.Name == "" {
		return dataset.KnowledgeSet{}, orig
	}
	prepared, err := cat.PrepareKnowledgeSet(setID, 1, rec.Sources())
	if err != nil {
		return dataset.KnowledgeSet{}, err
	}
	prepared.Metadata = rec.Metadata
	return cat.PublishKnowledgeSet(prepared)
}

func FindKnowledgeSetRecipe(ws *kchome.Home, home, setID string) (dataset.KnowledgeSetRecipe, error) {
	if raw, err := os.ReadFile(LocalWorkspaceFile(home, setID)); err == nil {
		rec, err := dataset.ParseKnowledgeSetRecipe(raw)
		if err != nil {
			return dataset.KnowledgeSetRecipe{}, err
		}
		if rec.Name == setID {
			return rec, nil
		}
	}
	var hits []dataset.KnowledgeSetRecipe
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
		return dataset.KnowledgeSetRecipe{}, nil
	}
	for i := 1; i < len(hits); i++ {
		a, _ := dataset.FormatKnowledgeSetRecipe(hits[0])
		b, _ := dataset.FormatKnowledgeSetRecipe(hits[i])
		if !bytes.Equal(a, b) {
			return dataset.KnowledgeSetRecipe{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid,
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

func ReadRecipeAtHead(ws *kchome.Home, id kernel.RepositoryID) (dataset.KnowledgeSetRecipe, bool) {
	snap, ok := ws.Store.Get(id)
	if !ok {
		return dataset.KnowledgeSetRecipe{}, false
	}
	files, ok := snapshot.TreeStoreOf(snap)
	if !ok {
		return dataset.KnowledgeSetRecipe{}, false
	}
	commit, err := snap.Head(snapshot.DefaultRef)
	if err != nil {
		return dataset.KnowledgeSetRecipe{}, false
	}
	raw, err := files.ReadFile(dataset.KnowledgeSetFileName, commit)
	if err != nil {
		return dataset.KnowledgeSetRecipe{}, false
	}
	rec, err := dataset.ParseKnowledgeSetRecipe(raw)
	if err != nil {
		return dataset.KnowledgeSetRecipe{}, false
	}
	return rec, true
}

func ApplyOverlay(home, as string, def dataset.KnowledgeSet) (dataset.KnowledgeSet, error) {
	raw, err := os.ReadFile(dataset.OverlayFile(home, as, def.SetID))
	if err != nil {
		if os.IsNotExist(err) {
			return def, nil
		}
		return dataset.KnowledgeSet{}, err
	}
	over, err := dataset.ParseKnowledgeSetOverlay(raw)
	if err != nil {
		return dataset.KnowledgeSet{}, err
	}
	return dataset.MergeOverlay(def, over)
}

func ResolveOrReplay(ws *kchome.Home, home string, cat dataset.Service, setID string, flags map[string]invoke.FlagValue) (dataset.ResolvedKnowledgeSet, error) {
	return ResolveKnowledgeSet(ws, home, cat, setID, flags)
}

func ResolveKnowledgeSet(ws *kchome.Home, home string, cat dataset.Service, setID string, flags map[string]invoke.FlagValue) (dataset.ResolvedKnowledgeSet, error) {
	var def dataset.KnowledgeSet
	if supplied := SuppliedKnowledgeSet(flags); supplied != nil {
		// Temporary definitions replay through the pin; a caller label must not
		// borrow a published Workspace grant when an explicit recipe is present.
		setID = ""
		def = *supplied
		if err := dataset.ValidateKnowledgeSet(def); err != nil {
			return dataset.ResolvedKnowledgeSet{}, err
		}
		for _, source := range def.Sources {
			if !cat.HasRepository(source.Repository) {
				return dataset.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "repository %s is not registered in this registry", source.Repository)
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
			return dataset.ResolvedKnowledgeSet{}, err
		}
	}
	if pinPath := invoke.FlagString(flags, "pin"); pinPath != "" {
		var resolved dataset.ResolvedKnowledgeSet
		pin, replayErr := ParseReplayPin(pinPath)
		if replayErr == nil {
			if SuppliedKnowledgeSet(flags) == nil {
				if pin.SetID != "" && pin.SetID != setID {
					return dataset.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "pin names a different dataset")
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

func ParseReplayPin(pinPath string) (dataset.ResolvedKnowledgeSet, error) {
	raw := []byte(pinPath)
	if !strings.HasPrefix(strings.TrimSpace(pinPath), "{") {
		var err error
		raw, err = os.ReadFile(pinPath)
		if err != nil {
			return dataset.ResolvedKnowledgeSet{}, err
		}
	}
	var pin dataset.ResolvedKnowledgeSet
	if err := dataset.DecodeJSON(raw, &pin); err != nil {
		return dataset.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin is not a ResolvedKnowledgeSet JSON file")
	}
	return pin, nil
}

func ReplayPin(cat dataset.Service, def dataset.KnowledgeSet, pinPath string) (dataset.ResolvedKnowledgeSet, error) {
	pin, err := DecodeReplayPin(def, pinPath)
	if err != nil {
		return dataset.ResolvedKnowledgeSet{}, err
	}
	if check := cat.CheckResolved(pin); check.Outcome != "PASSED" {
		return dataset.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "replayed pin failed CheckResolved")
	}
	return pin, nil
}

func DecodeReplayPin(def dataset.KnowledgeSet, pinPath string) (dataset.ResolvedKnowledgeSet, error) {
	pin, err := ParseReplayPin(pinPath)
	if err != nil {
		return dataset.ResolvedKnowledgeSet{}, err
	}
	return dataset.ReplayDefinition(def, pin)
}

func KnowledgeSetPin(resolved dataset.ResolvedKnowledgeSet) reader.KnowledgeSetPin {
	items := make([]reader.DatasetItem, 0, len(resolved.Items))
	for _, item := range resolved.Items {
		items = append(items, reader.DatasetItem{
			Target: item.Target, Repository: item.Repository, Commit: item.Commit,
			Kind: item.Kind, Prefix: item.Prefix, File: item.File,
		})
	}
	return reader.KnowledgeSetPin{SetID: resolved.SetID, Revision: resolved.Revision, Repositories: resolved.Repositories, Items: items}
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
	_, ok := MatchAllow(file.Rules, q)
	return ok
}

func SemanticMounts(def dataset.KnowledgeSet, pin dataset.ResolvedKnowledgeSet) []dataset.VirtualMount {
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
		mountPath := "knowledge/" + label
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

func SemanticProjectionFor(store snapshot.Store, repo objectmodel.Repository, pin dataset.ResolvedKnowledgeSet) (*SemanticProjection, error) {
	commit := pin.Repositories[repo.ID()]
	key := semanticProjectionKey{repository: repo.ID(), commit: commit, scope: kernel.CanonicalDigest(pin.Items)}
	semanticProjections.Lock()
	defer semanticProjections.Unlock()
	if cached := semanticProjections.entries[key]; cached != nil {
		return cached, nil
	}
	projection, err := BuildSemanticProjection(store, repo, pin)
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

func BuildSemanticProjection(store snapshot.Store, repo objectmodel.Repository, pin dataset.ResolvedKnowledgeSet) (*SemanticProjection, error) {
	commit := pin.Repositories[repo.ID()]
	scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, KnowledgeSetPin(pin))
	values := []objectmodel.KnowledgeValue{}
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
	if err := knowledgemaintenance.WalkScope(store, repo, commit, prefixes, files, func(id objectmodel.ObjectID) error {
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

func SemanticEntity(value objectmodel.KnowledgeValue, schemas map[objectmodel.ObjectID]objectmodel.SchemaDefinition) string {
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
	if allowedDatasetFileRead(home, flags, repository) {
		return true, nil
	}
	return PrincipalAllowed(home, invoke.FlagString(flags, "as"), "file.read", repository, opened...), nil
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
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "semantic file %s does not exist", file)
	}
	return bytes.Clone(content), nil
}

func DatasetFSRepositoryPath(subPath, relative string) (string, error) {
	return fileservice.RepositoryPath(subPath, relative)
}
func ValidKnowledgeSetFSRelative(value string) bool { return fileservice.ValidRelativePath(value) }

func RejectMixedKnowledgeBasis(flags map[string]invoke.FlagValue) error {
	if invoke.FlagString(flags, "dataset") == "" && SuppliedKnowledgeSet(flags) == nil && invoke.FlagString(flags, "pin") == "" && invoke.FlagString(flags, "dataset-file") == "" {
		return nil
	}
	if invoke.FlagString(flags, "repo") != "" || invoke.FlagString(flags, "commit") != "" || invoke.FlagString(flags, "ref") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "choose a Workspace definition/pin or a Repository basis; do not mix")
	}
	return nil
}
