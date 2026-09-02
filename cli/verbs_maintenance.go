package cli

func maintenanceVerbs() map[string]command {
	return map[string]command{
		"check-workspace": {stage: stageGoverned, run: verbCheckWorkspace},
	}
}

func verbCheckWorkspace(cx *invocation) (any, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	workspaceID, err := cx.workspaceID()
	if err != nil {
		return nil, err
	}
	resolved, err := resolveOrReplay(cx.WS, cx.Home, cat, workspaceID, cx.Flags)
	if err != nil {
		return nil, err
	}
	return cat.CheckResolved(resolved), nil
}
