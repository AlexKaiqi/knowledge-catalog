package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kc/catalog"
	apphome "kc/home"
	"kc/internal/journal"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledgeapp"
	"kc/snapshot"
)

// Catalog application operations admit repositories and publish Workspace
// recipes. Managed creation delegates allocation and an explicit initial
// grant policy to Home; ordinary attach and Workspace composition do not grant.
// catalog show lists repository ids; README is a knowledge object, not inventory
// title/summary. The catalog/ package still does not read knowledge.

func catalogVerbs() map[string]command {
	return map[string]command{
		"catalog-list":    {stage: stageHome, run: catalogListOperation},
		"show":            {stage: stageGoverned, run: readCatalogState},
		"dataset-define":  {stage: stageGoverned, run: verbDefineKnowledgeSet},
		"attach":          {stage: stageGoverned, run: verbRegister},
		"create":          {stage: stageGoverned, run: verbCreateManagedRepository},
		"dataset-retire":  {stage: stageGoverned, run: verbRetireKnowledgeSet},
		"catalog-archive": {stage: stageGoverned, run: verbArchiveCatalog},
		"detach":          {stage: stageGoverned, run: verbDetach},
		"catalog-use":     {stage: stageHome, run: catalogUseOperation},
	}
}

func catalogUseOperation(cx *invocation) (any, error) {
	catalogID, err := cx.require("catalog")
	if err != nil {
		return nil, err
	}
	if server := remoteServerURL(cx.Flags); server != "" {
		if err := persistClientCatalog(server, catalogID); err != nil {
			return nil, err
		}
	} else if cx.Home != "" {
		if err := persistHomeCatalog(cx.Home, catalogID); err != nil {
			return nil, err
		}
	}
	return map[string]any{"catalogId": catalogID}, nil
}

func verbDetach(cx *invocation) (any, error) {
	repositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	if repositoryID == string(knowledge.SystemRepositoryID) {
		return nil, kernel.Fail(kernel.ErrForbidden, "System Repository %s cannot be detached", repositoryID)
	}
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	if err := cat.UnregisterRepository(kernel.RepositoryID(repositoryID)); err != nil {
		return nil, err
	}
	return map[string]any{"catalogId": catalogIDOf(cx.WS, cx.Flags), "repositoryId": repositoryID, "detached": true}, nil
}

func verbCreateManagedRepository(cx *invocation) (any, error) {
	if err := validateManagedRepositoryCoordinates(cx.Flags); err != nil {
		return nil, err
	}
	if cx.WS == nil || cx.WS.Deployment == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "managed repository creation requires a declared deployment")
	}
	return cx.WS.CreateManagedRepository(apphome.ManagedRepositoryRequest{
		CatalogID: resolveCurrentCatalog(cx), RepositoryID: cx.flag("repo"), CommandID: cx.flag("command-id"), Principal: cx.flag("as"),
	}, func(grant apphome.ManagedRepositoryGrant) error {
		return ensureManagedRepositoryGrant(cx.Home, grant)
	})
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
		case "datasets":
			return map[string]any{"catalogId": typed.CatalogID, "datasets": publicKnowledgeSets(typed.KnowledgeSets)}, nil
		case "dataset":
			id := cx.flag("dataset")
			if id == "" {
				return nil, kernel.Fail(kernel.ErrUsageInvalid, "knowledge set show requires --dataset")
			}
			for _, set := range typed.KnowledgeSets {
				if set.SetID == id {
					view := publicKnowledgeSet(set)
					items := set.Items
					if items == nil {
						items = []catalog.DatasetItem{}
					}
					view["items"] = items
					return view, nil
				}
			}
			return nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "knowledge set %s is not visible", id)
		}
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown Catalog view")
	}
}

func verbDefineKnowledgeSet(cx *invocation) (any, error) {
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
	setID, err := defineSetID(cx, rec)
	if err != nil {
		return nil, err
	}
	sources, err = applyBaseRevs(sources, cx.flags("base-rev"))
	if err != nil {
		return nil, err
	}
	if err := authorize(cx.Home, "dataset.manage", cx.Flags, nil); err != nil {
		return nil, err
	}
	// Reject an invalid publication before writing its portable recipe. The
	// final freeze and publish recheck the revision after that independent write.
	if _, err := cat.PrepareKnowledgeSet(setID, revision, sources); err != nil {
		return nil, err
	}
	// Include the portable recipe in the frozen source commit. The retained
	// projection is then prepared at exactly that commit before acceptance.
	var published *recipePublish
	if !adopted {
		published, err = publishKnowledgeSetRecipe(cx, catalog.KnowledgeSet{
			SetID: setID, Revision: revision, Sources: sources,
		})
		if err != nil {
			return nil, err
		}
	}
	def, err := (knowledgeapp.DatasetPublisher{
		Registry: cat,
		Authorize: func(context.Context, knowledgeapp.DatasetPublication) error {
			return authorize(cx.Home, "dataset.manage", cx.Flags, nil)
		},
		Prepare: cx.WS.PrepareDataset,
	}).Execute(cx.Context, knowledgeapp.DatasetPublication{Dataset: setID, Revision: revision, Sources: sources})
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"setId":    def.SetID,
		"revision": def.Revision,
		"sources":  def.Sources,
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

func defineSetID(cx *invocation, rec catalog.KnowledgeSetRecipe) (string, error) {
	workspace := FlagString(cx.Flags, "dataset")
	if workspace != "" {
		return workspace, nil
	}
	if rec.Name != "" {
		return rec.Name, nil
	}
	return "", fmt.Errorf("missing --dataset")
}

func workspaceSources(cx *invocation) ([]catalog.KnowledgeSetSource, catalog.KnowledgeSetRecipe, bool, error) {
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
		return nil, catalog.KnowledgeSetRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --source, --file, --from-repo, or typed payload")
	}
	if payload != "" {
		var sources []catalog.KnowledgeSetSource
		if err := json.Unmarshal([]byte(payload), &sources); err != nil || len(sources) == 0 {
			return nil, catalog.KnowledgeSetRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid, "typed Workspace payload must contain sources")
		}
		return sources, catalog.KnowledgeSetRecipe{}, false, nil
	}
	if fromRepo != "" {
		rec, ok := readRecipeAtHead(cx.WS, kernel.RepositoryID(fromRepo))
		if !ok {
			return nil, catalog.KnowledgeSetRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid, "%s has no %s at %s", fromRepo, catalog.KnowledgeSetFileName, snapshot.DefaultRef)
		}
		return rec.Sources(), rec, true, nil
	}
	if file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, catalog.KnowledgeSetRecipe{}, false, err
		}
		rec, err := catalog.ParseKnowledgeSetRecipe(raw)
		if err != nil {
			return nil, catalog.KnowledgeSetRecipe{}, false, err
		}
		return rec.Sources(), rec, false, nil
	}
	sources, err := workspaceSourcesFrom(items)
	if err != nil {
		return nil, catalog.KnowledgeSetRecipe{}, false, err
	}
	return sources, catalog.KnowledgeSetRecipe{}, false, nil
}

// workspaceSourcesFrom parses --source <repository>[=selector][@path[@subPath]].
// Callers who only know a knowledge source id omit the selector; the published
// default is filled in here so they never have to name a Snapshot ref.
//
// The @path suffix is what makes a source a mount (catalog.KnowledgeSetSource.Path
// is *string so "declared as root" and "not declared" are different states):
// omit @ entirely for a pure federated-read source (Path stays nil); write
// @ with nothing after it for the root mount (Path: ""); write @refs/x for a
// nested mount; add a second @ for SubPath (@kb@docs/knowledge mounts only
// docs/knowledge from the member, at workspace path kb).
func workspaceSourcesFrom(items []string) ([]catalog.KnowledgeSetSource, error) {
	var sources []catalog.KnowledgeSetSource
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
		src := catalog.KnowledgeSetSource{Repository: kernel.RepositoryID(repo), Selector: selector}
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
	repositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	if err := cx.WS.AttachRepository(resolveCurrentCatalog(cx), kernel.RepositoryID(repositoryID)); err != nil {
		return nil, err
	}
	return map[string]any{"catalog": catalogIDOf(cx.WS, cx.Flags), "repositoryId": repositoryID}, nil
}

func verbRetireKnowledgeSet(cx *invocation) (any, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	setID, err := cx.setID()
	if err != nil {
		return nil, err
	}
	if err := cat.RetireKnowledgeSet(setID); err != nil {
		return nil, err
	}
	return map[string]any{"dataset": setID, "retired": true}, nil
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

func applyBaseRevs(sources []catalog.KnowledgeSetSource, items []string) ([]catalog.KnowledgeSetSource, error) {
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
	out := append([]catalog.KnowledgeSetSource{}, sources...)
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
	setID, err := cx.setID()
	if err != nil {
		return nil, err
	}
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	path := catalog.OverlayFile(cx.Home, cx.flag("as"), setID)
	if FlagBool(cx.Flags, "clear") {
		if cx.flag("file") != "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --file or --clear")
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		return map[string]any{"setId": setID, "cleared": true, "file": path}, nil
	}
	if file := cx.flag("file"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		over, err := catalog.ParseKnowledgeSetOverlay(raw)
		if err != nil {
			return nil, err
		}
		def, err := ensureWorkspace(cx.WS, cx.Home, cat, setID)
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
			"setId":   setID,
			"file":    path,
			"sources": merged.Sources,
		}, nil
	}
	def, err := effectiveWorkspace(cx.WS, cx.Home, cat, setID, cx.Flags)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	out := map[string]any{"setId": setID, "file": path, "sources": def.Sources}
	if err == nil {
		over, parseErr := catalog.ParseKnowledgeSetOverlay(raw)
		if parseErr != nil {
			return nil, parseErr
		}
		out["overlay"] = over
	}
	return out, nil
}

func pickCatalog(ws *Home, flags map[string]FlagValue) (*catalog.Catalog, error) {
	catalogID := FlagString(flags, "catalog")
	if catalogID == "" && ws != nil {
		catalogID = savedHomeCatalog(ws.Dir)
		if catalogID != "" {
			flags["catalog"] = catalogID
		}
	}
	cat, _, err := ws.UseCatalog(catalogID)
	return cat, err
}

// resolveCurrentCatalog returns the Catalog an embedded operation targets,
// applying the same priority as the remote client: an explicit operand wins,
// then the choice persisted by `kc catalog use`, then the Home default.
func resolveCurrentCatalog(cx *invocation) string {
	catalogID := FlagString(cx.Flags, "catalog")
	if catalogID == "" && cx.Home != "" {
		if saved := savedHomeCatalog(cx.Home); saved != "" {
			cx.Flags["catalog"] = saved
			catalogID = saved
		}
	}
	return catalogID
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
	var file HomeFile
	var err error
	if cx.WS != nil {
		file = cx.WS.File
	} else {
		file, err = ReadHome(cx.Home)
	}
	if err != nil {
		return nil, err
	}
	visible := make([]catalogInventoryItem, 0, len(file.Catalogs))
	for _, item := range file.Catalogs {
		if ownerBypass(cx.Flags) || catalogReadAllowed(cx.Home, FlagString(cx.Flags, "as"), item.ID, cx.WS) {
			visible = append(visible, catalogInventoryItem{ID: item.ID})
		}
	}
	return map[string]any{"catalogs": visible}, nil
}

// readCatalogState answers `kc catalog show`: the current combination space,
// not git history (`kc catalog audit`) or local stores (`kc deployment status`).
// The workspaces list member ids only. Registered repositories stay identity
// plus schemaCount; README is not flattened into title/summary.
func readCatalogState(cx *invocation) (any, error) {
	state, err := loadVisibleCatalogState(cx)
	if err != nil {
		return nil, err
	}
	view := publicCatalogView(state)
	if id := configuredDiscoveryWorkspace(cx.WS, state.CatalogID); id != "" {
		view["discoveryWorkspaceId"] = id
	}
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
		"datasets":     publicKnowledgeSets(state.KnowledgeSets),
	}
	if state.Archived {
		out["archived"] = true
	}
	return out
}

func publicKnowledgeSets(workspaces []catalog.KnowledgeSet) []map[string]any {
	out := make([]map[string]any, 0, len(workspaces))
	for _, workspace := range workspaces {
		out = append(out, publicKnowledgeSet(workspace))
	}
	return out
}

func publicKnowledgeSet(workspace catalog.KnowledgeSet) map[string]any {
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
		"id":           workspace.SetID,
		"revision":     workspace.Revision,
		"repositories": repos,
		"itemCount":    len(workspace.Items),
	}
	if workspace.Retired {
		out["retired"] = true
	}
	return out
}
