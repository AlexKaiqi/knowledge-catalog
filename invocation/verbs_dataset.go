package invocation

import (
	"encoding/json"
	"fmt"
	discoverydatasets "kc/discovery/datasets"
	journal "kc/internal/journal"
	objectservice "kc/object/service"
	"os"
	"path/filepath"
	"strings"

	"bytes"
	"context"
	"crypto/sha256"

	kchome "kc/home"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"

	"kc/snapshot"
	"kc/snapshot/dataset"

	"strconv"
)

func ReadInventoryStatePart(part string) Handler {
	return func(cx *Invocation) (any, error) {
		typed, err := LoadInventoryState(cx)
		if err != nil {
			return nil, err
		}
		switch part {
		case "repositories":
			return map[string]any{"repositories": RepositoryInventory(cx.WS, typed.Repositories)}, nil
		case "datasets":
			return map[string]any{"datasets": PublicKnowledgeSets(typed.KnowledgeSets)}, nil
		case "dataset":
			id := cx.Flag("dataset")
			if id == "" {
				return nil, kernel.Fail(kernel.ErrUsageInvalid, "knowledge set show requires --dataset")
			}
			revision := 0
			if raw := cx.Flag("revision"); raw != "" {
				var err error
				revision, err = strconv.Atoi(raw)
				if err != nil || revision <= 0 {
					return nil, kernel.Fail(kernel.ErrUsageInvalid, "revision must be a positive integer")
				}
			}
			return datasetManagementView(typed, id, revision, cx.Flags)
		}
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown inventory view")
	}
}

func VerbArchiveRepo(cx *Invocation) (any, error) {
	repositoryID, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	if repositoryID == string(objectmodel.SystemRepositoryID) {
		return nil, kernel.Fail(kernel.ErrForbidden, "System Repository %s cannot be archived", repositoryID)
	}
	repo, err := RequireRepo(cx.WS, repositoryID)
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

func VerbCreateManagedRepository(cx *Invocation) (any, error) {
	if err := ValidateManagedRepositoryCoordinates(cx.Flags); err != nil {
		return nil, err
	}
	if cx.WS == nil || cx.WS.Deployment == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "managed repository creation requires a declared deployment")
	}
	return cx.WS.CreateManagedRepository(kchome.ManagedRepositoryRequest{
		RepositoryID: cx.Flag("repo"), CommandID: cx.Flag("command-id"), Principal: cx.Flag("as"),
	}, func(grant kchome.ManagedRepositoryGrant) error {
		return EnsureManagedRepositoryGrant(cx.Home, grant)
	})
}

func VerbDefineKnowledgeSet(cx *Invocation) (any, error) {
	cat, err := PickDatasets(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	sources, rec, adopted, err := WorkspaceSources(cx)
	if err != nil {
		return nil, err
	}
	revision, err := DefineRevision(cx)
	if err != nil {
		return nil, err
	}
	setID, err := DefineSetID(cx, rec)
	if err != nil {
		return nil, err
	}
	metadata, err := DatasetMetadata(cx.Flags, rec.Metadata)
	if err != nil {
		return nil, err
	}
	sources, err = ApplyBaseRevs(sources, cx.FlagValues("base-rev"))
	if err != nil {
		return nil, err
	}
	if err := Authorize(cx.Home, "dataset.manage", cx.Flags, nil); err != nil {
		return nil, err
	}
	// Reject an invalid publication before writing its portable recipe. The
	// final freeze and publish recheck the revision after that independent write.
	if _, err := cat.PrepareKnowledgeSet(setID, revision, sources); err != nil {
		return nil, err
	}
	// Include the portable recipe in the frozen source commit. The retained
	// projection is then prepared at exactly that commit before acceptance.
	var published *RecipePublish
	if !adopted {
		published, err = PublishKnowledgeSetRecipe(cx, dataset.KnowledgeSet{
			SetID: setID, Revision: revision, Sources: sources, Metadata: metadata,
		})
		if err != nil {
			return nil, err
		}
	}
	def, err := (discoverydatasets.DatasetPublisher{
		Registry: cat,
		Authorize: func(context.Context, discoverydatasets.DatasetPublication) error {
			return Authorize(cx.Home, "dataset.manage", cx.Flags, nil)
		},
		Prepare: cx.WS.DatasetPreparation.Execute,
	}).Execute(cx.Context, discoverydatasets.DatasetPublication{Dataset: setID, Revision: revision, Sources: sources, Metadata: metadata})
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"setId":    def.SetID,
		"revision": def.Revision,
		"sources":  def.Sources,
	}
	if len(def.Metadata) > 0 {
		out["metadata"] = def.Metadata
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

func VerbRegister(cx *Invocation) (any, error) {
	repositoryID, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	if err := cx.WS.AttachRepository(kernel.RepositoryID(repositoryID)); err != nil {
		return nil, err
	}
	return map[string]any{"repositoryId": repositoryID}, nil
}

func VerbRetireKnowledgeSet(cx *Invocation) (any, error) {
	cat, err := PickDatasets(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	setID, err := cx.SetID()
	if err != nil {
		return nil, err
	}
	if err := cat.RetireKnowledgeSet(setID); err != nil {
		return nil, err
	}
	return map[string]any{"dataset": setID, "retired": true}, nil
}

func DefineRevision(cx *Invocation) (int, error) {
	raw := cx.Flag("revision")
	if raw == "" {
		if cx.Flag("file") != "" || cx.Flag("from-repo") != "" {
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

func DefineSetID(cx *Invocation, rec dataset.KnowledgeSetRecipe) (string, error) {
	workspace := invoke.FlagString(cx.Flags, "dataset")
	if workspace != "" {
		return workspace, nil
	}
	if rec.Name != "" {
		return rec.Name, nil
	}
	return "", fmt.Errorf("missing --dataset")
}

func ApplyBaseRevs(sources []dataset.KnowledgeSetSource, items []string) ([]dataset.KnowledgeSetSource, error) {
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
	out := append([]dataset.KnowledgeSetSource{}, sources...)
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

func PublishKnowledgeSetRecipe(cx *Invocation, def dataset.KnowledgeSet) (*RecipePublish, error) {
	rec, ok := dataset.RecipeFromKnowledgeSet(def)
	if !ok {
		return nil, nil
	}
	raw, err := dataset.FormatKnowledgeSetRecipe(rec)
	if err != nil {
		return nil, err
	}
	if root := dataset.RootMount(def.Sources); root != nil {
		return WriteRecipeToRepo(cx, *root, raw, def.SetID, def.Revision)
	}
	if err := os.MkdirAll(filepath.Join(cx.Home, "datasets"), 0o755); err != nil {
		return nil, err
	}
	path := LocalWorkspaceFile(cx.Home, def.SetID)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return nil, err
	}
	return &RecipePublish{File: path, Location: "local"}, nil
}

type RecipePublish struct {
	File       string `json:"recipeFile,omitempty"`
	Repository string `json:"recipeRepository,omitempty"`
	Commit     string `json:"recipeCommit,omitempty"`
	Location   string `json:"recipeLocation,omitempty"`
	Skipped    string `json:"recipeSkipped,omitempty"`
}

func WorkspaceSources(cx *Invocation) ([]dataset.KnowledgeSetSource, dataset.KnowledgeSetRecipe, bool, error) {
	file := cx.Flag("file")
	fromRepo := cx.Flag("from-repo")
	items := cx.FlagValues("source")
	payload := cx.Flag("payload")
	fileEntries, err := dataset.ParseFileMappings(cx.FlagValues("file-source"))
	if err != nil {
		return nil, dataset.KnowledgeSetRecipe{}, false, err
	}
	if len(fileEntries) > 0 && (file != "" || fromRepo != "" || payload != "") {
		return nil, dataset.KnowledgeSetRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid,
			"--file-source combines with --source only; a recipe file carries file entries in its files section")
	}
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
		return nil, dataset.KnowledgeSetRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --source, --file, --from-repo, or typed payload")
	}
	if payload != "" {
		var sources []dataset.KnowledgeSetSource
		if err := json.Unmarshal([]byte(payload), &sources); err != nil || len(sources) == 0 {
			return nil, dataset.KnowledgeSetRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid, "typed Workspace payload must contain sources")
		}
		return sources, dataset.KnowledgeSetRecipe{}, false, nil
	}
	if fromRepo != "" {
		rec, ok := ReadRecipeAtHead(cx.WS, kernel.RepositoryID(fromRepo))
		if !ok {
			return nil, dataset.KnowledgeSetRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid, "%s has no %s at %s", fromRepo, dataset.KnowledgeSetFileName, snapshot.DefaultRef)
		}
		return rec.Sources(), rec, true, nil
	}
	if file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, dataset.KnowledgeSetRecipe{}, false, err
		}
		rec, err := dataset.ParseKnowledgeSetRecipe(raw)
		if err != nil {
			return nil, dataset.KnowledgeSetRecipe{}, false, err
		}
		return rec.Sources(), rec, false, nil
	}
	var sources []dataset.KnowledgeSetSource
	if len(items) > 0 {
		sources, err = dataset.ParseSourceMappings(items)
		if err != nil {
			return nil, dataset.KnowledgeSetRecipe{}, false, err
		}
	}
	sources = append(sources, fileEntries...)
	if len(sources) == 0 {
		return nil, dataset.KnowledgeSetRecipe{}, false, kernel.Fail(kernel.ErrUsageInvalid, "at least one --source or --file-source is required")
	}
	return sources, dataset.KnowledgeSetRecipe{}, false, nil
}

func WriteRecipeToRepo(cx *Invocation, root dataset.KnowledgeSetSource, raw []byte, setID string, revision int) (*RecipePublish, error) {
	if err := AuthorizeRoutedWrite(cx, root.Repository); err != nil {
		if kernel.CodeOf(err) == kernel.ErrForbidden {
			return &RecipePublish{File: dataset.KnowledgeSetFileName, Repository: string(root.Repository), Skipped: "no write grant for root mount"}, nil
		}
		return nil, err
	}
	snap, ok := cx.WS.Store.Get(root.Repository)
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown repository %s", root.Repository)
	}
	files, ok := snapshot.TreeStoreOf(snap)
	if !ok {
		return &RecipePublish{File: dataset.KnowledgeSetFileName, Repository: string(root.Repository), Skipped: "root mount does not support raw path writes"}, nil
	}
	ref := snapshot.RefOrDefault(root.Selector)
	head, err := snap.Head(ref)
	if err != nil {
		return nil, err
	}
	if existing, err := files.ReadFile(dataset.KnowledgeSetFileName, head); err == nil && bytes.Equal(existing, raw) {
		return &RecipePublish{File: dataset.KnowledgeSetFileName, Repository: string(root.Repository), Commit: string(head), Location: "repository"}, nil
	}
	sum := sha256.Sum256(raw)
	commandID := fmt.Sprintf("dataset-define:%s:%d:%x", setID, revision, sum[:8])
	changes := []snapshot.TreeChange{{Path: dataset.KnowledgeSetFileName, Content: raw}}
	receipt, err := cx.WS.TreeWriter.Commit(commandID, snapshot.TreeChangeSet{
		TargetRepository:     root.Repository,
		TargetRef:            ref,
		BaseCommit:           head,
		ExpectedTargetCommit: head,
		Changes:              changes,
		Message:              "workspace " + setID,
	})
	if err != nil {
		return nil, fmt.Errorf("%s was not written for workspace %s: %w", dataset.KnowledgeSetFileName, setID, err)
	}
	return &RecipePublish{
		File:       dataset.KnowledgeSetFileName,
		Repository: string(root.Repository),
		Commit:     string(receipt.Result.NewCommit),
		Location:   "repository",
	}, nil
}

// authorizeRoutedWrite checks a Writer operation after a Workspace recipe has
// selected the real target Repository. It is not a VFS write capability.
func AuthorizeRoutedWrite(cx *Invocation, target kernel.RepositoryID) error {
	scoped := make(map[string]invoke.FlagValue, len(cx.Flags)+1)
	for key, value := range cx.Flags {
		if key == "dataset" {
			continue
		}
		scoped[key] = value
	}
	scoped["repo"] = string(target)
	return Authorize(cx.Home, "writer.commit", scoped, cx.Observation.Authorization)
}

func VerbOverlay(cx *Invocation) (any, error) {
	setID, err := cx.SetID()
	if err != nil {
		return nil, err
	}
	cat, err := PickDatasets(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	path := dataset.OverlayFile(cx.Home, cx.Flag("as"), setID)
	if invoke.FlagBool(cx.Flags, "clear") {
		if cx.Flag("file") != "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --file or --clear")
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		return map[string]any{"setId": setID, "cleared": true, "file": path}, nil
	}
	if file := cx.Flag("file"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		over, err := dataset.ParseKnowledgeSetOverlay(raw)
		if err != nil {
			return nil, err
		}
		def, err := EnsureWorkspace(cx.WS, cx.Home, cat, setID)
		if err != nil {
			return nil, err
		}
		merged, err := dataset.MergeOverlay(def, over)
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
	def, err := EffectiveWorkspace(cx.WS, cx.Home, cat, setID, cx.Flags)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	out := map[string]any{"setId": setID, "file": path, "sources": def.Sources}
	if err == nil {
		over, parseErr := dataset.ParseKnowledgeSetOverlay(raw)
		if parseErr != nil {
			return nil, parseErr
		}
		out["overlay"] = over
	}
	return out, nil
}

// VerbDatasetCloneRequiresServer keeps the public verb honest outside a
// deployment: clone walks the Server File Gateway, so a local kchome.Home without a
// Server has nothing to consume (the file-system projection and clone are
// deployment consumption paths).
func VerbDatasetCloneRequiresServer(cx *Invocation) (any, error) {
	return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
		"kc dataset clone consumes a KC Server deployment; pass --server or set KC_SERVER_URL")
}

func VerbDetach(cx *Invocation) (any, error) {
	repositoryID, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	if repositoryID == string(objectmodel.SystemRepositoryID) {
		return nil, kernel.Fail(kernel.ErrForbidden, "System Repository %s cannot be detached", repositoryID)
	}
	cat, err := PickDatasets(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	if err := cat.UnregisterRepository(kernel.RepositoryID(repositoryID)); err != nil {
		return nil, err
	}
	return map[string]any{"repositoryId": repositoryID, "detached": true}, nil
}

// effectiveWorkspace is the recipe this command actually uses: Registry (or a
// hitchhiking yaml) plus the local overlay for this --as, if any.
func EffectiveWorkspace(ws *kchome.Home, home string, cat dataset.Service, setID string, flags map[string]invoke.FlagValue) (dataset.KnowledgeSet, error) {
	def, err := EnsureWorkspace(ws, home, cat, setID)
	if err != nil {
		return dataset.KnowledgeSet{}, err
	}
	if ws.Deployment != nil {
		return def, nil
	}
	return ApplyOverlay(home, invoke.FlagString(flags, "as"), def)
}

func RemoteServerURL(flags map[string]invoke.FlagValue) string {
	if value := strings.TrimSpace(invoke.FlagString(flags, "server")); value != "" {
		return value
	}
	if server := strings.TrimSpace(os.Getenv("KC_SERVER_URL")); server != "" {
		return server
	}
	return SavedClientServer()
}

// VerbBrowseSchemas names the entities published at one Repository basis.
func VerbBrowseSchemas(cx *Invocation) (any, error) {
	repositoryID, commitID, err := PinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	limit, err := PageLimit(cx.Flags, DefaultSchemaPageSize, MaxSchemaPageSize)
	if err != nil {
		return nil, err
	}
	repo, err := cx.WS.Reader.Require(repositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return nil, err
	}
	return (objectservice.SchemaBrowseExecutor{Repository: repo}).Execute(cx.Context, objectservice.SchemaBrowseRequest{Commit: commitID, Limit: limit, Continuation: cx.Flag("continuation")})
}
