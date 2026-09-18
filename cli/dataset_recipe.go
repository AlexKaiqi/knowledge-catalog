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

// ensureWorkspace returns the Catalog recipe for setID, adopting a hitchhiking
// .kc-dataset.yaml from an attached member (or a local unanchored copy) when
// this machine has never defined it. Catalog remains the operational store;
// the yaml is how the recipe travels with git (docs/COMPOSITION.md §1.4).
func ensureWorkspace(ws *Home, home string, cat *catalog.Catalog, setID string) (catalog.KnowledgeSet, error) {
	def, orig := cat.Set(setID)
	if ws.Deployment != nil {
		return def, orig
	}
	if orig == nil {
		return def, nil
	}
	rec, err := findKnowledgeSetRecipe(ws, home, setID)
	if err != nil {
		return catalog.KnowledgeSet{}, err
	}
	if rec.Name == "" {
		return catalog.KnowledgeSet{}, orig
	}
	return cat.DefineKnowledgeSet(setID, 1, rec.Sources())
}

// effectiveWorkspace is the recipe this command actually uses: Catalog (or a
// hitchhiking yaml) plus the local overlay for this --as, if any.
func effectiveWorkspace(ws *Home, home string, cat *catalog.Catalog, setID string, flags map[string]FlagValue) (catalog.KnowledgeSet, error) {
	def, err := ensureWorkspace(ws, home, cat, setID)
	if err != nil {
		return catalog.KnowledgeSet{}, err
	}
	if ws.Deployment != nil {
		return def, nil
	}
	return applyOverlay(home, FlagString(flags, "as"), def)
}

func applyOverlay(home, as string, def catalog.KnowledgeSet) (catalog.KnowledgeSet, error) {
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

func findKnowledgeSetRecipe(ws *Home, home, setID string) (catalog.KnowledgeSetRecipe, error) {
	if raw, err := os.ReadFile(localWorkspaceFile(home, setID)); err == nil {
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
		rec, ok := readRecipeAtHead(ws, id)
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

func readRecipeAtHead(ws *Home, id kernel.RepositoryID) (catalog.KnowledgeSetRecipe, bool) {
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

func localWorkspaceFile(home, name string) string {
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return filepath.Join(home, "datasets", safe+".yaml")
}

type recipePublish struct {
	File       string `json:"recipeFile,omitempty"`
	Repository string `json:"recipeRepository,omitempty"`
	Commit     string `json:"recipeCommit,omitempty"`
	Location   string `json:"recipeLocation,omitempty"`
	Skipped    string `json:"recipeSkipped,omitempty"`
}

func publishKnowledgeSetRecipe(cx *invocation, def catalog.KnowledgeSet) (*recipePublish, error) {
	rec, ok := catalog.RecipeFromKnowledgeSet(def)
	if !ok {
		return nil, nil
	}
	raw, err := catalog.FormatKnowledgeSetRecipe(rec)
	if err != nil {
		return nil, err
	}
	if root := catalog.RootMount(def.Sources); root != nil {
		return writeRecipeToRepo(cx, *root, raw, def.SetID, def.Revision)
	}
	if err := os.MkdirAll(filepath.Join(cx.Home, "datasets"), 0o755); err != nil {
		return nil, err
	}
	path := localWorkspaceFile(cx.Home, def.SetID)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return nil, err
	}
	return &recipePublish{File: path, Location: "local"}, nil
}

func writeRecipeToRepo(cx *invocation, root catalog.KnowledgeSetSource, raw []byte, setID string, revision int) (*recipePublish, error) {
	if err := authorizeRoutedWrite(cx, root.Repository); err != nil {
		if kernel.CodeOf(err) == kernel.ErrForbidden {
			return &recipePublish{File: catalog.KnowledgeSetFileName, Repository: string(root.Repository), Skipped: "no write grant for root mount"}, nil
		}
		return nil, err
	}
	snap, ok := cx.WS.Store.Get(root.Repository)
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "unknown repository %s", root.Repository)
	}
	files, ok := snapshot.TreeStoreOf(snap)
	if !ok {
		return &recipePublish{File: catalog.KnowledgeSetFileName, Repository: string(root.Repository), Skipped: "root mount does not support raw path writes"}, nil
	}
	ref := snapshot.RefOrDefault(root.Selector)
	head, err := snap.Head(ref)
	if err != nil {
		return nil, err
	}
	if existing, err := files.ReadFile(catalog.KnowledgeSetFileName, head); err == nil && bytes.Equal(existing, raw) {
		return &recipePublish{File: catalog.KnowledgeSetFileName, Repository: string(root.Repository), Commit: string(head), Location: "repository"}, nil
	}
	sum := sha256.Sum256(raw)
	commandID := fmt.Sprintf("dataset-define:%s:%d:%x", setID, revision, sum[:8])
	changes := []snapshot.TreeChange{{Path: catalog.KnowledgeSetFileName, Content: raw}}
	receipt, err := cx.WS.TreeWriter.Commit(commandID, snapshot.TreeChangeSet{
		TargetRepository:     root.Repository,
		TargetRef:            ref,
		BaseCommit:           head,
		ExpectedTargetCommit: head,
		Changes:              changes,
		Message:              "workspace " + setID,
	})
	if err != nil {
		return nil, fmt.Errorf("%s was not written for workspace %s: %w", catalog.KnowledgeSetFileName, setID, err)
	}
	return &recipePublish{
		File:       catalog.KnowledgeSetFileName,
		Repository: string(root.Repository),
		Commit:     string(receipt.Result.NewCommit),
		Location:   "repository",
	}, nil
}
