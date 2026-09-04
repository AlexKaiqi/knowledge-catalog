package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kc/catalog"
	"kc/internal/journal"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// Composition verbs (layer ①). These admit repositories and publish Workspace
// recipes; none of them grants permission (that is `kc admin grant add`).
// catalog show / repository list assemble source profiles through Knowledge
// Reader; the catalog/ package still does not read knowledge.

func catalogVerbs() map[string]command {
	return map[string]command{
		"catalog-list":         {stage: stageHome, run: catalogListOperation},
		"catalog-show":         {stage: stageGoverned, run: readCatalogState},
		"catalog-repositories": {stage: stageGoverned, run: readCatalogStatePart("repositories")},
		"catalog-workspaces":   {stage: stageGoverned, run: readCatalogStatePart("workspaces")},
		"catalog-workspace":    {stage: stageGoverned, run: readCatalogStatePart("workspace")},
		"define-workspace":     {stage: stageGoverned, run: verbDefineWorkspace},
		"overlay":              {stage: stageOpen, run: verbOverlay},
		"register":             {stage: stageGoverned, run: verbRegister},
		"retire-workspace":     {stage: stageGoverned, run: verbRetireWorkspace},
		"archive-catalog":      {stage: stageGoverned, run: verbArchiveCatalog},
		"archive-repo":         {stage: stageGoverned, run: verbArchiveRepo},
	}
}

func readCatalogStatePart(part string) handler {
	return func(cx *invocation) (any, error) {
		typed, err := loadVisibleCatalogState(cx)
		if err != nil {
			return nil, err
		}
		switch part {
		case "repositories":
			return map[string]any{"catalogId": typed.CatalogID, "repositories": catalogRepositoryInventory(cx.WS, typed.Repositories)}, nil
		case "workspaces":
			return map[string]any{"catalogId": typed.CatalogID, "workspaces": publicWorkspaceDefinitions(typed.Workspaces)}, nil
		case "workspace":
			id := cx.flag("workspace")
			if id == "" {
				return nil, kernel.Fail(kernel.ErrUsageInvalid, "catalog workspace show requires --workspace")
			}
			for _, workspace := range typed.Workspaces {
				if workspace.WorkspaceID == id {
					return publicWorkspaceDefinition(workspace), nil
				}
			}
			return nil, kernel.Fail(kernel.ErrWorkspaceInvalid, "workspace %s is not visible", id)
		}
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown Catalog view")
	}
}

func verbDefineWorkspace(cx *invocation) (any, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	sources, rec, adopted, err := workspaceSources(cx)
	if err != nil {
		return nil, err
	}
	revision, err := defineRevision(cx)
	if err != nil {
		return nil, err
	}
	workspaceID, err := defineWorkspaceID(cx, rec)
	if err != nil {
		return nil, err
	}
	sources, err = applyBaseRevs(sources, cx.flags("base-rev"))
	if err != nil {
		return nil, err
	}
	def, err := cat.DefineWorkspace(workspaceID, revision, sources)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"workspaceId": def.WorkspaceID,
		"revision":    def.Revision,
		"sources":     def.Sources,
	}
	if adopted {
		return out, nil
	}
	published, err := publishWorkspaceRecipe(cx, def)
	if err != nil {
		return nil, err
	}
	if published != nil {
		if published.File != "" {
			out["recipeFile"] = published.File
		}
		if published.Repository != "" {
			out["recipeRepository"] = published.Repository
		}
		if published.Commit != "" {
			out["recipeCommit"] = published.Commit
		}
		if published.Location != "" {
			out["recipeLocation"] = published.Location
		}
		if published.Skipped != "" {
			out["recipeSkipped"] = published.Skipped
		}
	}
	return out, nil
}

func defineRevision(cx *invocation) (int, error) {
	raw := cx.flag("revision")
	if raw == "" {
		if cx.flag("file") != "" || cx.flag("from-repo") != "" {
			return 1, nil
		}
		return 0, fmt.Errorf("missing --revision")
	}
	revision, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("--revision must be a number")
	}
	return revision, nil
}

func defineWorkspaceID(cx *invocation, rec catalog.WorkspaceRecipe) (string, error) {
	workspace := FlagString(cx.Flags, "workspace")
	if workspace != "" {
		return workspace, nil
	}
	if rec.Name != "" {
		return rec.Name, nil
	}
	return "", fmt.Errorf("missing --workspace")
}

func workspaceSources(cx *invocation) ([]catalog.WorkspaceSource, catalog.WorkspaceRecipe, bool, error) {
	file := cx.flag("file")
	fromRepo := cx.flag("from-repo")
	items := cx.flags("source")
	payload := cx.flag("payload")
	n := 0
	if file != "" {
		n++
	}
	if fromRepo != "" {
		n++
	}
	if len(items) > 0 {
		n++
	}
	if payload != "" {
		n++
	}
	if n > 1 {
		return nil, catalog.WorkspaceRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --source, --file, --from-repo, or typed payload")
	}
	if payload != "" {
		var sources []catalog.WorkspaceSource
		if err := json.Unmarshal([]byte(payload), &sources); err != nil || len(sources) == 0 {
			return nil, catalog.WorkspaceRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid, "typed Workspace payload must contain sources")
		}
		return sources, catalog.WorkspaceRecipe{}, false, nil
	}
	if fromRepo != "" {
		rec, ok := readRecipeAtHead(cx.WS, kernel.RepositoryID(fromRepo))
		if !ok {
			return nil, catalog.WorkspaceRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid, "%s has no %s at %s", fromRepo, catalog.WorkspaceFileName, snapshot.DefaultRef)
		}
		return rec.Sources(), rec, true, nil
	}
	if file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, catalog.WorkspaceRecipe{}, false, err
		}
		rec, err := catalog.ParseWorkspaceRecipe(raw)
		if err != nil {
			return nil, catalog.WorkspaceRecipe{}, false, err
		}
		return rec.Sources(), rec, false, nil
	}
	sources, err := workspaceSourcesFrom(items)
	if err != nil {
		return nil, catalog.WorkspaceRecipe{}, false, err
	}
	return sources, catalog.WorkspaceRecipe{}, false, nil
}

// workspaceSourcesFrom parses --source <repository>[=selector][@path[@subPath]].
// Callers who only know a knowledge source id omit the selector; the published
// default is filled in here so they never have to name a Snapshot ref.
//
// The @path suffix is what makes a source a mount (catalog.WorkspaceSource.Path
// is *string so "declared as root" and "not declared" are different states):
// omit @ entirely for a pure federated-read source (Path stays nil); write
// @ with nothing after it for the root mount (Path: ""); write @refs/x for a
// nested mount; add a second @ for SubPath (@kb@docs/knowledge mounts only
// docs/knowledge from the member, at workspace path kb).
func workspaceSourcesFrom(items []string) ([]catalog.WorkspaceSource, error) {
	var sources []catalog.WorkspaceSource
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("--source requires a knowledge source id")
		}
		repoSelector, rest, hasPath := strings.Cut(item, "@")
		repo, selector, ok := strings.Cut(repoSelector, "=")
		if !ok {
			repo = repoSelector
			selector = snapshot.DefaultRef
		} else if strings.TrimSpace(selector) == "" {
			selector = snapshot.DefaultRef
		}
		repo = strings.TrimSpace(repo)
		if repo == "" {
			return nil, fmt.Errorf("--source must be <repository>[=selector][@path[@subPath]], got %s", item)
		}
		src := catalog.WorkspaceSource{Repository: kernel.RepositoryID(repo), Selector: selector}
		if hasPath {
			path, subPath, _ := strings.Cut(rest, "@")
			src.Path = catalog.MountPath(path)
			src.SubPath = subPath
		}
		sources = append(sources, src)
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("at least one --source <repository>[=selector][@path] is required")
	}
	return sources, nil
}

func verbRegister(cx *invocation) (any, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	repositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	if _, ok := cx.WS.Store.Get(kernel.RepositoryID(repositoryID)); !ok {
		return nil, fmt.Errorf("unknown repository %s; run repo-add first", repositoryID)
	}
	if err := cat.RegisterRepository(kernel.RepositoryID(repositoryID)); err != nil {
		return nil, err
	}
	return map[string]any{"catalog": catalogIDOf(cx.WS, cx.Flags), "repositoryId": repositoryID}, nil
}

func verbRetireWorkspace(cx *invocation) (any, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	workspaceID, err := cx.workspaceID()
	if err != nil {
		return nil, err
	}
	if err := cat.RetireWorkspace(workspaceID); err != nil {
		return nil, err
	}
	return map[string]any{"workspace": workspaceID, "retired": true}, nil
}

func verbArchiveCatalog(cx *invocation) (any, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	if err := cat.Archive(); err != nil {
		return nil, err
	}
	return map[string]any{"catalog": catalogIDOf(cx.WS, cx.Flags), "archived": true}, nil
}

func verbArchiveRepo(cx *invocation) (any, error) {
	repositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	if repositoryID == string(knowledge.SystemRepositoryID) {
		return nil, kernel.Fail(kernel.ErrForbidden, "System Repository %s cannot be archived", repositoryID)
	}
	repo, err := requireRepo(cx.WS, repositoryID)
	if err != nil {
		return nil, err
	}
	if err := repo.Archive(); err != nil {
		return nil, err
	}
	if err := journal.Finish(cx.WS.Journal, journal.LayerSystem, "repository", "archive-repo",
		map[string]any{"repositoryId": repositoryID}, nil); err != nil {
		return nil, err
	}
	return map[string]any{"repositoryId": repositoryID, "archived": true}, nil
}

func applyBaseRevs(sources []catalog.WorkspaceSource, items []string) ([]catalog.WorkspaceSource, error) {
	if len(items) == 0 {
		return sources, nil
	}
	byRepo := map[kernel.RepositoryID]string{}
	for _, item := range items {
		repo, rev, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(repo) == "" || strings.TrimSpace(rev) == "" {
			return nil, fmt.Errorf("--base-rev must be repo=commit, got %s", item)
		}
		rid := kernel.RepositoryID(repo)
		if _, dup := byRepo[rid]; dup {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "--base-rev names repository %s twice", rid)
		}
		byRepo[rid] = rev
	}
	out := append([]catalog.WorkspaceSource{}, sources...)
	hit := map[kernel.RepositoryID]bool{}
	for i := range out {
		if rev, ok := byRepo[out[i].Repository]; ok {
			out[i].BaseRev = rev
			hit[out[i].Repository] = true
		}
	}
	for repo := range byRepo {
		if !hit[repo] {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "--base-rev names repository %s which is not a workspace source", repo)
		}
	}
	return out, nil
}

func verbOverlay(cx *invocation) (any, error) {
	workspaceID, err := cx.workspaceID()
	if err != nil {
		return nil, err
	}
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	path := catalog.OverlayFile(cx.Home, cx.flag("as"), workspaceID)
	if FlagBool(cx.Flags, "clear") {
		if cx.flag("file") != "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --file or --clear")
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		return map[string]any{"workspaceId": workspaceID, "cleared": true, "file": path}, nil
	}
	if file := cx.flag("file"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		over, err := catalog.ParseWorkspaceOverlay(raw)
		if err != nil {
			return nil, err
		}
		def, err := ensureWorkspace(cx.WS, cx.Home, cat, workspaceID)
		if err != nil {
			return nil, err
		}
		merged, err := catalog.MergeOverlay(def, over)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return nil, err
		}
		return map[string]any{
			"workspaceId": workspaceID,
			"file":        path,
			"sources":     merged.Sources,
		}, nil
	}
	def, err := effectiveWorkspace(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	out := map[string]any{"workspaceId": workspaceID, "file": path, "sources": def.Sources}
	if err == nil {
		over, parseErr := catalog.ParseWorkspaceOverlay(raw)
		if parseErr != nil {
			return nil, parseErr
		}
		out["overlay"] = over
	}
	return out, nil
}

func pickCatalog(ws *Home, flags map[string]FlagValue) (*catalog.Catalog, error) {
	cat, _, err := ws.UseCatalog(FlagString(flags, "catalog"))
	return cat, err
}

func catalogIDOf(ws *Home, flags map[string]FlagValue) string {
	if id := FlagString(flags, "catalog"); id != "" {
		return id
	}
	if len(ws.File.Catalogs) > 0 {
		return ws.File.Catalogs[0].ID
	}
	return ""
}

type catalogInventoryItem struct {
	ID string `json:"id"`
}

// catalogListOperation is DiscoverCatalogs: visible Catalog IDs only. Host
// paths stay on the Server; consumers never receive them.
func catalogListOperation(cx *invocation) (any, error) {
	file, err := ReadHome(cx.Home)
	if err != nil {
		return nil, err
	}
	visible := make([]catalogInventoryItem, 0, len(file.Catalogs))
	for _, item := range file.Catalogs {
		if ownerBypass(cx.Flags) || PrincipalAllowed(cx.Home, FlagString(cx.Flags, "as"), "catalog.read", "", item.ID) {
			visible = append(visible, catalogInventoryItem{ID: item.ID})
		}
	}
	return map[string]any{"catalogs": visible}, nil
}

// readCatalogState answers `kc catalog show`: the current combination space,
// not git history (`kc catalog audit`) or local stores (`kc local status`).
// The workspaces list member ids only. Registered repositories are assembled
// with source profiles by the application layer.
func readCatalogState(cx *invocation) (any, error) {
	state, err := loadVisibleCatalogState(cx)
	if err != nil {
		return nil, err
	}
	view := publicCatalogView(state)
	view["repositories"] = catalogRepositoryInventory(cx.WS, state.Repositories)
	return view, nil
}

func loadVisibleCatalogState(cx *invocation) (catalog.CatalogState, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return catalog.CatalogState{}, err
	}
	return filterCatalogState(cx.Home, cx.Flags, cat.DumpState()), nil
}

func publicCatalogView(state catalog.CatalogState) map[string]any {
	state = catalog.NormalizeCatalogState(state)
	out := map[string]any{
		"catalogId":    state.CatalogID,
		"repositories": state.Repositories,
		"workspaces":   publicWorkspaceDefinitions(state.Workspaces),
	}
	if state.Archived {
		out["archived"] = true
	}
	return out
}

func publicWorkspaceDefinitions(workspaces []catalog.WorkspaceDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(workspaces))
	for _, workspace := range workspaces {
		out = append(out, publicWorkspaceDefinition(workspace))
	}
	return out
}

func publicWorkspaceDefinition(workspace catalog.WorkspaceDefinition) map[string]any {
	repos := make([]string, 0, len(workspace.Sources))
	seen := map[string]bool{}
	for _, src := range workspace.Sources {
		id := string(src.Repository)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		repos = append(repos, id)
	}
	out := map[string]any{
		"workspaceId":  workspace.WorkspaceID,
		"revision":     workspace.Revision,
		"repositories": repos,
	}
	if workspace.Retired {
		out["retired"] = true
	}
	return out
}
