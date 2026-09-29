package invocation

import (
	"fmt"

	kchome "kc/home"
	"kc/kernel"
)

// VerbInit bootstraps one local Home: init the home file and the system
// repository for one catalog id. (CAP-ORG-01 B6.)
func VerbInit(cx *Invocation) (any, error) {
	if cx.Flag("namespace") != "" {
		return nil, fmt.Errorf("init takes --catalog <id> (kr://acme/catalog or acme/catalog), not --namespace")
	}
	catalogID := cx.Flag("catalog")
	file, _, err := kchome.InitHome(cx.Home, catalogID)
	if err != nil {
		return nil, err
	}
	id := catalogID
	if id != "" {
		id, err = kchome.NormalizeCatalogID(id)
		if err != nil {
			return nil, err
		}
	} else if len(file.Catalogs) > 0 {
		id = file.Catalogs[0].ID
	}
	systemCommit, err := kchome.EnsureSystemRepository(cx.Home, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"catalog": id, "system": SystemRepositoryStatus(systemCommit)}, nil
}

func SystemRepositoryStatus(commit kernel.CommitID) map[string]any {
	return kchome.SystemRepositoryStatus(commit)
}
