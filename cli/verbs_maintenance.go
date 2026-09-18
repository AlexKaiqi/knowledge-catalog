package cli

func maintenanceVerbs() map[string]command {
	return map[string]command{
		"pin-check": {stage: stageGoverned, run: verbCheckKnowledgeSet},
	}
}

func verbCheckKnowledgeSet(cx *invocation) (any, error) {
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	setID, err := cx.setID()
	if err != nil {
		return nil, err
	}
	resolved, err := resolveOrReplay(cx.WS, cx.Home, cat, setID, cx.Flags)
	if err != nil {
		return nil, err
	}
	return cat.CheckResolved(resolved), nil
}
