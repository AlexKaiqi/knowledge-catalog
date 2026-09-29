package invocation

// The dataset access layer: catalog selection, workspace recipes, knowledge-set
// resolution and replay pins, semantic views, and the file-read authorization
// that both the argv consume path and the HTTP file service run through.
// (CAP-ORG-01 B6: shared execution moves into the seam.)

import (
	"bytes"
	"encoding/json"
	"os"
	"path"

	"gopkg.in/yaml.v3"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"kc/catalog"
	kchome "kc/home"
	"kc/invoke"
	"kc/kernel"
	knowledge "kc/knowledge"
	knowledgemaintenance "kc/knowledge/maintenance"
	reader "kc/knowledge/reader"
	"kc/knowledge/semanticview"
	"kc/snapshot"
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

type ClientCatalogState struct {
	Catalog string `json:"catalog"`
}

func PickCatalog(ws *kchome.Home, flags map[string]invoke.FlagValue) (*catalog.Catalog, error) {
	catalogID := invoke.FlagString(flags, "catalog")
	if catalogID == "" && ws != nil {
		catalogID = SavedHomeCatalog(ws.Dir)
		if catalogID != "" {
			flags["catalog"] = catalogID
		}
	}
	cat, _, err := ws.UseCatalog(catalogID)
	return cat, err
}

func SavedHomeCatalog(home string) string {
	raw, err := os.ReadFile(HomeCatalogUsePath(home))
	if err != nil {
		return ""
	}
	var state ClientCatalogState
	if json.Unmarshal(raw, &state) != nil {
		return ""
	}
	return strings.TrimSpace(state.Catalog)
}

func HomeCatalogUsePath(home string) string {
	return filepath.Join(home, "catalog-use.json")
}

// EnsureWorkspace returns the Catalog recipe for setID, adopting a hitchhiking
// .kc-dataset.yaml from an attached member (or a local unanchored copy) when
// this machine has never defined it. Catalog remains the operational store;
// the yaml is how the recipe travels with git (docs/reviewed/dataset.md).
func EnsureWorkspace(ws *kchome.Home, home string, cat *catalog.Catalog, setID string) (catalog.KnowledgeSet, error) {
	def, orig := cat.Set(setID)
	if ws.Deployment != nil {
		return def, orig
	}
	if orig == nil {
		return def, nil
	}
	rec, err := FindKnowledgeSetRecipe(ws, home, setID)
	if err != nil {
		return catalog.KnowledgeSet{}, err
	}
	if rec.Name == "" {
		return catalog.KnowledgeSet{}, orig
	}
	return cat.DefineKnowledgeSet(setID, 1, rec.Sources())
}

func FindKnowledgeSetRecipe(ws *kchome.Home, home, setID string) (catalog.KnowledgeSetRecipe, error) {
	if raw, err := os.ReadFile(LocalWorkspaceFile(home, setID)); err == nil {
		rec, err := catalog.ParseKnowledgeSetRecipe(raw)
		if err != nil {
			return catalog.KnowledgeSetRecipe{}, err
		}
		if rec.Name == setID {
			return rec, nil
		}
	}
	var hits []catalog.KnowledgeSetRecipe
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
		return catalog.KnowledgeSetRecipe{}, nil
	}
	for i := 1; i < len(hits); i++ {
		a, _ := catalog.FormatKnowledgeSetRecipe(hits[0])
		b, _ := catalog.FormatKnowledgeSetRecipe(hits[i])
		if !bytes.Equal(a, b) {
			return catalog.KnowledgeSetRecipe{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid,
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

func ReadRecipeAtHead(ws *kchome.Home, id kernel.RepositoryID) (catalog.KnowledgeSetRecipe, bool) {
	snap, ok := ws.Store.Get(id)
	if !ok {
		return catalog.KnowledgeSetRecipe{}, false
	}
	files, ok := snapshot.TreeStoreOf(snap)
	if !ok {
		return catalog.KnowledgeSetRecipe{}, false
	}
	commit, err := snap.Head(snapshot.DefaultRef)
	if err != nil {
		return catalog.KnowledgeSetRecipe{}, false
	}
	raw, err := files.ReadFile(catalog.KnowledgeSetFileName, commit)
	if err != nil {
		return catalog.KnowledgeSetRecipe{}, false
	}
	rec, err := catalog.ParseKnowledgeSetRecipe(raw)
	if err != nil {
		return catalog.KnowledgeSetRecipe{}, false
	}
	return rec, true
}

func ApplyOverlay(home, as string, def catalog.KnowledgeSet) (catalog.KnowledgeSet, error) {
	raw, err := os.ReadFile(catalog.OverlayFile(home, as, def.SetID))
	if err != nil {
		if os.IsNotExist(err) {
			return def, nil
		}
		return catalog.KnowledgeSet{}, err
	}
	over, err := catalog.ParseKnowledgeSetOverlay(raw)
	if err != nil {
		return catalog.KnowledgeSet{}, err
	}
	return catalog.MergeOverlay(def, over)
}

func ResolveOrReplay(ws *kchome.Home, home string, cat *catalog.Catalog, setID string, flags map[string]invoke.FlagValue) (catalog.ResolvedKnowledgeSet, error) {
	return ResolveKnowledgeSet(ws, home, cat, setID, flags)
}

func ResolveKnowledgeSet(ws *kchome.Home, home string, cat *catalog.Catalog, setID string, flags map[string]invoke.FlagValue) (catalog.ResolvedKnowledgeSet, error) {
	var def catalog.KnowledgeSet
	if supplied := SuppliedKnowledgeSet(flags); supplied != nil {
		// Temporary definitions replay through the pin; a caller label must not
		// borrow a published Workspace grant when an explicit recipe is present.
		setID = ""
		def = *supplied
		if err := catalog.ValidateKnowledgeSet(def); err != nil {
			return catalog.ResolvedKnowledgeSet{}, err
		}
		for _, source := range def.Sources {
			if !cat.HasRepository(source.Repository) {
				return catalog.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "repository %s is not registered in this catalog", source.Repository)
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
			return catalog.ResolvedKnowledgeSet{}, err
		}
	}
	if pinPath := invoke.FlagString(flags, "pin"); pinPath != "" {
		var resolved catalog.ResolvedKnowledgeSet
		pin, replayErr := ParseReplayPin(pinPath)
		if replayErr == nil {
			if SuppliedKnowledgeSet(flags) == nil {
				if pin.SetID != "" && pin.SetID != setID {
					return catalog.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "pin names a different dataset")
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

func ParseReplayPin(pinPath string) (catalog.ResolvedKnowledgeSet, error) {
	raw := []byte(pinPath)
	if !strings.HasPrefix(strings.TrimSpace(pinPath), "{") {
		var err error
		raw, err = os.ReadFile(pinPath)
		if err != nil {
			return catalog.ResolvedKnowledgeSet{}, err
		}
	}
	var pin catalog.ResolvedKnowledgeSet
	if err := catalog.DecodeJSON(raw, &pin); err != nil {
		return catalog.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "--pin is not a ResolvedKnowledgeSet JSON file")
	}
	return pin, nil
}

func ReplayPin(cat *catalog.Catalog, def catalog.KnowledgeSet, pinPath string) (catalog.ResolvedKnowledgeSet, error) {
	pin, err := DecodeReplayPin(def, pinPath)
	if err != nil {
		return catalog.ResolvedKnowledgeSet{}, err
	}
	if check := cat.CheckResolved(pin); check.Outcome != "PASSED" {
		return catalog.ResolvedKnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "replayed pin failed CheckResolved")
	}
	return pin, nil
}

func DecodeReplayPin(def catalog.KnowledgeSet, pinPath string) (catalog.ResolvedKnowledgeSet, error) {
	pin, err := ParseReplayPin(pinPath)
	if err != nil {
		return catalog.ResolvedKnowledgeSet{}, err
	}
	return catalog.ReplayDefinition(def, pin)
}

func KnowledgeSetPin(resolved catalog.ResolvedKnowledgeSet) reader.KnowledgeSetPin {
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
		Catalog:   invoke.FlagString(flags, "catalog"),
		Dataset:   invoke.DatasetOf(flags),
	}
	if q.Catalog == "" {
		q.Catalog = invoke.FlagString(flags, "_default-catalog")
	}
	if q.Catalog == "" {
		q.Catalog = DefaultAllowCatalog(home, "")
	}
	_, ok := MatchAllow(file.Rules, q)
	return ok
}

func SemanticMounts(def catalog.KnowledgeSet, pin catalog.ResolvedKnowledgeSet) []catalog.VirtualMount {
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
			Selector: selectors[repository], SubPath: SemanticFileViewV1,
		})
	}
	return mounts
}

func SemanticProjectionFor(store snapshot.Store, repo knowledge.Repository, pin catalog.ResolvedKnowledgeSet) (*SemanticProjection, error) {
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

func BuildSemanticProjection(store snapshot.Store, repo knowledge.Repository, pin catalog.ResolvedKnowledgeSet) (*SemanticProjection, error) {
	commit := pin.Repositories[repo.ID()]
	scope := reader.Open(func(kernel.RepositoryID) (knowledge.Repository, error) { return repo, nil }, KnowledgeSetPin(pin))
	values := []knowledge.KnowledgeValue{}
	var prefixes, files []string
	for _, item := range pin.Items {
		if item.Repository != repo.ID() || item.Commit != commit {
			continue
		}
		if item.Kind == catalog.DatasetItemPrefix {
			prefixes = append(prefixes, item.Prefix)
		}
		if item.Kind == catalog.DatasetItemFile {
			files = append(files, item.File)
		}
	}
	if err := knowledgemaintenance.WalkScope(store, repo, commit, prefixes, files, func(id knowledge.ObjectID) error {
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

func SemanticEntity(value knowledge.KnowledgeValue, schemas map[knowledge.ObjectID]knowledge.SchemaDefinition) string {
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

func DatasetFSMayReadRepository(home string, flags map[string]invoke.FlagValue, repository string, opened ...*kchome.Home) (bool, error) {
	if OwnerBypass(flags) {
		return true, nil
	}
	if AllowedDatasetFileRead(home, flags) {
		return true, nil
	}
	catalogID := invoke.FlagString(flags, "catalog")
	if catalogID == "" {
		catalogID = invoke.FlagString(flags, "_default-catalog")
	}
	return PrincipalAllowed(home, invoke.FlagString(flags, "as"), "file.read", repository, catalogID, opened...), nil
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
	joined := strings.Trim(path.Join(strings.Trim(subPath, "/"), strings.Trim(relative, "/")), "/")
	if relative != "" && (!ValidKnowledgeSetFSRelative(relative) || joined == "") {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "invalid mount-relative path %q", relative)
	}
	// Normalization must not move the result outside the selected subtree:
	// a relative path whose parent-directory segments clean away the subPath
	// prefix (e.g. "scratch/../..") escapes the published file list even
	// though it never begins with "../" (docs/reviewed/dataset.md, C-21).
	if subPath != "" && joined != "" && joined != strings.Trim(subPath, "/") &&
		!strings.HasPrefix(joined, strings.Trim(subPath, "/")+"/") {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "mount-relative path %q escapes the selected subtree", relative)
	}
	return joined, nil
}

func ValidKnowledgeSetFSRelative(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || strings.ContainsRune(value, '\x00') {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func RejectMixedKnowledgeBasis(flags map[string]invoke.FlagValue) error {
	if invoke.FlagString(flags, "dataset") == "" && SuppliedKnowledgeSet(flags) == nil && invoke.FlagString(flags, "pin") == "" && invoke.FlagString(flags, "dataset-file") == "" {
		return nil
	}
	if invoke.FlagString(flags, "repo") != "" || invoke.FlagString(flags, "commit") != "" || invoke.FlagString(flags, "ref") != "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "choose a Workspace definition/pin or a Repository basis; do not mix")
	}
	return nil
}
