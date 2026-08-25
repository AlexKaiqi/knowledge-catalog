package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kc/catalog"
	"kc/kernel"
	"kc/snapshot"
)

// ensureWorkspace returns the Catalog recipe for workspaceID, adopting a hitchhiking
// .kc-workspace.yaml from a mounted member (or a local unanchored copy) when
// this machine has never defined it. Catalog remains the operational store;
// the yaml is how the recipe travels with git (docs/COMPOSITION.md §1.4).
func ensureWorkspace(ws *Home, home string, cat *catalog.Catalog, workspaceID string) (catalog.WorkspaceDefinition, error) {
	def, orig := cat.Workspace(workspaceID)
	if orig == nil {
		return def, nil
	}
	rec, err := findWorkspaceRecipe(ws, home, workspaceID)
	if err != nil {
		return catalog.WorkspaceDefinition{}, err
	}
	if rec.Name == "" {
		return catalog.WorkspaceDefinition{}, orig
	}
	return cat.DefineWorkspace(workspaceID, 1, rec.Sources())
}

// effectiveWorkspace is the recipe this command actually uses: Catalog (or a
// hitchhiking yaml) plus the local overlay for this --as, if any.
func effectiveWorkspace(ws *Home, home string, cat *catalog.Catalog, workspaceID string, flags map[string]FlagValue) (catalog.WorkspaceDefinition, error) {
	def, err := ensureWorkspace(ws, home, cat, workspaceID)
	if err != nil {
		return catalog.WorkspaceDefinition{}, err
	}
	return applyOverlay(home, FlagString(flags, "as"), def)
}

func applyOverlay(home, as string, def catalog.WorkspaceDefinition) (catalog.WorkspaceDefinition, error) {
	raw, err := os.ReadFile(catalog.OverlayFile(home, as, def.WorkspaceID))
	if err != nil {
		if os.IsNotExist(err) {
			return def, nil
		}
		return catalog.WorkspaceDefinition{}, err
	}
	over, err := catalog.ParseWorkspaceOverlay(raw)
	if err != nil {
		return catalog.WorkspaceDefinition{}, err
	}
	return catalog.MergeOverlay(def, over)
}

func findWorkspaceRecipe(ws *Home, home, workspaceID string) (catalog.WorkspaceRecipe, error) {
	if raw, err := os.ReadFile(localWorkspaceFile(home, workspaceID)); err == nil {
		rec, err := catalog.ParseWorkspaceRecipe(raw)
		if err != nil {
			return catalog.WorkspaceRecipe{}, err
		}
		if rec.Name == workspaceID {
			return rec, nil
		}
	}
	var hits []catalog.WorkspaceRecipe
	var from []kernel.RepositoryID
	for _, id := range ws.Store.IDs() {
		rec, ok := readRecipeAtHead(ws, id)
		if !ok || rec.Name != workspaceID {
			continue
		}
		hits = append(hits, rec)
		from = append(from, id)
	}
	if len(hits) == 0 {
		return catalog.WorkspaceRecipe{}, nil
	}
	for i := 1; i < len(hits); i++ {
		a, _ := catalog.FormatWorkspaceRecipe(hits[0])
		b, _ := catalog.FormatWorkspaceRecipe(hits[i])
		if !bytes.Equal(a, b) {
			return catalog.WorkspaceRecipe{}, kernel.Fail(kernel.ErrWorkspaceInvalid,
				"workspace %s is declared differently in %s and %s", workspaceID, from[0], from[i])
		}
	}
	return hits[0], nil
}

func readRecipeAtHead(ws *Home, id kernel.RepositoryID) (catalog.WorkspaceRecipe, bool) {
	snap, ok := ws.Store.Get(id)
	if !ok {
		return catalog.WorkspaceRecipe{}, false
	}
	files, ok := snapshot.TreeStoreOf(snap)
	if !ok {
		return catalog.WorkspaceRecipe{}, false
	}
	commit, err := snap.Head(snapshot.DefaultRef)
	if err != nil {
		return catalog.WorkspaceRecipe{}, false
	}
	raw, err := files.ReadFile(catalog.WorkspaceFileName, commit)
	if err != nil {
		return catalog.WorkspaceRecipe{}, false
	}
	rec, err := catalog.ParseWorkspaceRecipe(raw)
	if err != nil {
		return catalog.WorkspaceRecipe{}, false
	}
	return rec, true
}

func localWorkspaceFile(home, name string) string {
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return filepath.Join(home, "workspaces", safe+".yaml")
}

type recipePublish struct {
	File       string `json:"recipeFile,omitempty"`
	Repository string `json:"recipeRepository,omitempty"`
	Commit     string `json:"recipeCommit,omitempty"`
	Location   string `json:"recipeLocation,omitempty"`
	Skipped    string `json:"recipeSkipped,omitempty"`
}

func publishWorkspaceRecipe(cx *invocation, def catalog.WorkspaceDefinition) (*recipePublish, error) {
	rec, ok := catalog.RecipeFromWorkspace(def)
	if !ok {
		return nil, nil
	}
	raw, err := catalog.FormatWorkspaceRecipe(rec)
	if err != nil {
		return nil, err
	}
	if root := catalog.RootMount(def.Sources); root != nil {
		return writeRecipeToRepo(cx, *root, raw, def.WorkspaceID, def.Revision)
	}
	if err := os.MkdirAll(filepath.Join(cx.Home, "workspaces"), 0o755); err != nil {
		return nil, err
	}
	path := localWorkspaceFile(cx.Home, def.WorkspaceID)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return nil, err
	}
	return &recipePublish{File: path, Location: "local"}, nil
}

func writeRecipeToRepo(cx *invocation, root catalog.WorkspaceSource, raw []byte, workspaceID string, revision int) (*recipePublish, error) {
	if err := authorizeRoutedWrite(cx, root.Repository); err != nil {
		if kernel.CodeOf(err) == kernel.ErrForbidden {
			return &recipePublish{File: catalog.WorkspaceFileName, Repository: string(root.Repository), Skipped: "no write grant for root mount"}, nil
		}
		return nil, err
	}
	snap, ok := cx.WS.Store.Get(root.Repository)
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown repository %s", root.Repository)
	}
	files, ok := snapshot.TreeStoreOf(snap)
	if !ok {
		return &recipePublish{File: catalog.WorkspaceFileName, Repository: string(root.Repository), Skipped: "root mount does not support raw path writes"}, nil
	}
	ref := snapshot.RefOrDefault(root.Selector)
	head, err := snap.Head(ref)
	if err != nil {
		return nil, err
	}
	if existing, err := files.ReadFile(catalog.WorkspaceFileName, head); err == nil && bytes.Equal(existing, raw) {
		return &recipePublish{File: catalog.WorkspaceFileName, Repository: string(root.Repository), Commit: string(head), Location: "repository"}, nil
	}
	sum := sha256.Sum256(raw)
	commandID := fmt.Sprintf("define-workspace:%s:%d:%x", workspaceID, revision, sum[:8])
	receipt, err := cx.WS.Writer.RawWrite(commandID, snapshot.TreeChangeSet{
		TargetRepository:     root.Repository,
		TargetRef:            ref,
		BaseCommit:           head,
		ExpectedTargetCommit: head,
		Changes:              []snapshot.TreeChange{{Path: catalog.WorkspaceFileName, Content: raw}},
		Message:              "workspace " + workspaceID,
	})
	if err != nil {
		return nil, fmt.Errorf("workspace %s defined in the catalog but %s was not written: %w", workspaceID, catalog.WorkspaceFileName, err)
	}
	return &recipePublish{
		File:       catalog.WorkspaceFileName,
		Repository: string(root.Repository),
		Commit:     string(receipt.Result.NewCommit),
		Location:   "repository",
	}, nil
}
