package cli

import (
	"os"
	"path/filepath"

	"kc/catalog"
	"kc/kernel"
	"kc/knowledge/reader"
)

func checkoutWorkspace(ws *Home, home string, flags map[string]FlagValue) (any, error) {
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return nil, err
	}
	workspaceID, err := workspaceIDFlag(flags)
	if err != nil {
		return nil, err
	}
	def, err := effectiveWorkspace(ws, home, cat, workspaceID, flags)
	if err != nil {
		return nil, err
	}
	dest, err := checkoutDest(ws, home, workspaceID, flags)
	if err != nil {
		return nil, err
	}
	if recipeHasMounts(def) {
		return checkoutMountsWorkspace(ws, home, flags, cat, def, dest)
	}
	return checkoutKnowledgeWorkspace(ws, home, flags, dest)
}

func recipeHasMounts(def catalog.WorkspaceDefinition) bool {
	return catalog.HasMountPaths(def.Sources)
}

func checkoutDest(ws *Home, home, workspaceID string, flags map[string]FlagValue) (string, error) {
	if to := FlagString(flags, "to"); to != "" {
		if filepath.IsAbs(to) {
			return to, nil
		}
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, to), nil
	}
	root, err := resolveStoreDir(home, ws.Stores.Layout.Checkouts, defaultCheckoutsDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, reader.EncodeCheckoutDir(workspaceID)), nil
}

func deniedMounts(home string, flags map[string]FlagValue, def catalog.WorkspaceDefinition) map[kernel.RepositoryID]string {
	out := map[kernel.RepositoryID]string{}
	as := FlagString(flags, "as")
	for _, src := range def.Sources {
		if allowedRepoRead(home, flags, string(src.Repository), "") {
			continue
		}
		reason := "not allowed to read " + string(src.Repository)
		if as != "" {
			reason = as + " is not allowed to read " + string(src.Repository)
		}
		out[src.Repository] = reason
	}
	return out
}

func checkoutMountsWorkspace(ws *Home, home string, flags map[string]FlagValue, cat *catalog.Catalog, def catalog.WorkspaceDefinition, dest string) (any, error) {
	denied := deniedMounts(home, flags, def)
	mounts, err := cat.CheckoutMountsAllowingDef(def, dest, denied)
	if err != nil {
		return nil, err
	}
	return map[string]any{"workspaceId": def.WorkspaceID, "dir": homeRel(home, dest), "mounts": mounts}, nil
}

func checkoutKnowledgeWorkspace(ws *Home, home string, flags map[string]FlagValue, dest string) (any, error) {
	serving, cat, err := openServing(ws, flags)
	if err != nil {
		return nil, err
	}
	values, err := serving.List()
	if err != nil {
		return nil, err
	}
	values = filterWorkspaceReads(home, flags, cat, values)
	report, err := reader.WriteCheckout(dest, serving.Pin(), values)
	if err != nil {
		return nil, err
	}
	report.Dir = homeRel(home, dest)
	return withKnowledgeEvidence(report, values), nil
}
