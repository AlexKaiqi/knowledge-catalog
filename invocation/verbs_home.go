package invocation

import (
	"kc/home"
	"kc/kernel"
)

func VerbInit(cx *Invocation) (any, error) {
	file, _, err := home.InitHome(cx.Home, home.RegistryBinding{ID: cx.Flag("repo"), Driver: cx.Flag("driver"), DSN: cx.Flag("dsn")})
	if err != nil {
		return nil, err
	}
	commit, err := home.EnsureSystemRepository(cx.Home)
	if err != nil {
		return nil, err
	}
	return map[string]any{"authorityId": file.Registry.ID, "system": SystemRepositoryStatus(commit)}, nil
}
func SystemRepositoryStatus(commit kernel.CommitID) map[string]any {
	return home.SystemRepositoryStatus(commit)
}
