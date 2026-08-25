package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kc/catalog"
	"kc/internal/journal"
	"kc/kernel"
	"kc/snapshot"
)

// Composition verbs (layer ①). These admit repositories and publish Workspace
// recipes; none of them grants permission (that is `kc allow`) and none of them
// reads knowledge.

func catalogVerbs() map[string]command {
	return map[string]command{
		"define-workspace": {stage: stageGoverned, run: verbDefineWorkspace},
		"overlay":          {stage: stageOpen, run: verbOverlay},
		"register":         {stage: stageGoverned, run: verbRegister},
		"retire-workspace": {stage: stageGoverned, run: verbRetireWorkspace},
		"archive-catalog":  {stage: stageGoverned, run: verbArchiveCatalog},
		"archive-repo":     {stage: stageGoverned, run: verbArchiveRepo},
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
	if n > 1 {
		return nil, catalog.WorkspaceRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --source, --file, or --from-repo")
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

// workspaceSourcesFrom parses --source repo=selector[@path[@subPath]]. The
// selector is a published ref that ResolveWorkspace maps to a commit once per
// command; it is not a commit.
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
		repoSelector, rest, hasPath := strings.Cut(item, "@")
		repo, selector, ok := strings.Cut(repoSelector, "=")
		if !ok {
			return nil, fmt.Errorf("--source must be repo=selector[@path[@subPath]], got %s", item)
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
		return nil, fmt.Errorf("define-workspace requires at least one --source repo=selector[@path]")
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

// readCatalogState answers `kc read --catalog`: the current combination space,
// not git history (that is `kc audit`) and not local stores (that is `kc status`).
func readCatalogState(cx *invocation) (any, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	return filterCatalogState(cx.Home, cx.Flags, cat.DumpState()), nil
}
