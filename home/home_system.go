package home

import (
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/writer"
	"kc/snapshot"
)

// ensureSystemRepository is host bootstrap orchestration. Registry Core sees
// only a Repository ID; the application root verifies the published Snapshot
// against the binary trust root.
func EnsureSystemRepository(home string) (kernel.CommitID, error) {
	ws, err := Open(home)
	if err != nil {
		return "", err
	}
	defer ws.Close()

	repo, exists := ws.Store.Get(knowledge.SystemRepositoryID)
	if !exists {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "published System Repository is not mounted")
	}
	cat := ws.Datasets
	if err := cat.RegisterRepository(knowledge.SystemRepositoryID); err != nil {
		return "", err
	}
	if err := EnsureRepositoryAccess(home, SystemRepositoryAccess()); err != nil {
		return "", err
	}

	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		return "", err
	}
	knowledgeRepo, err := ws.Reader.Require(knowledge.SystemRepositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return "", err
	}
	if err := verifySystemPublication(knowledgeRepo, repo, head); err != nil {
		return "", err
	}
	return head, nil
}

func SystemRepositoryStatus(commit kernel.CommitID) map[string]any {
	return map[string]any{
		"repositoryId":     knowledge.SystemRepositoryID,
		"commit":           commit,
		"metaSchema":       knowledge.MetaSchemaV1,
		"metaSchemaDigest": knowledge.SystemMetaSchemaDigest(),
	}
}

func PublishSystemRepository(home, driver, dsn, dir string) (map[string]any, error) {
	driver = strings.TrimSpace(driver)
	if driver == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "system publish requires --driver lakefs")
	}
	driver = normalizeRepoDriver(driver)
	authority, err := authorityFor(driver)
	if err != nil {
		return nil, err
	}
	stores, err := ReadStores(home)
	if err != nil {
		return nil, err
	}
	file, err := ReadHome(home)
	if err != nil {
		return nil, err
	}
	spec := repoAddRequest{
		ID:     string(knowledge.SystemRepositoryID),
		Driver: driver,
		DSN:    strings.TrimSpace(dsn),
		Dir:    strings.TrimSpace(dir),
	}
	if existing, ok := systemAuthoritySpec(file); ok {
		if existing.Driver != "" && normalizeRepoDriver(existing.Driver) != driver {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed,
				"%s is already published on driver %s", knowledge.SystemRepositoryID, existing.Driver)
		}
		if existing.DSN != "" && spec.DSN != "" && existing.DSN != spec.DSN {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed,
				"%s is already published at %s", knowledge.SystemRepositoryID, existing.DSN)
		}
		if spec.DSN == "" {
			spec.DSN = existing.DSN
		}
		// Reuse DSN only. Home-relative Dir must go through ResolveStoreDir(home),
		// not from the current working directory.
	}
	if driver == "lakefs" && spec.DSN == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "%s system publish requires --dsn", driver)
	}
	item, err := authority.prepare(stores, spec)
	if err != nil {
		return nil, err
	}
	item.ID = string(knowledge.SystemRepositoryID)
	abs, err := ResolveStoreDir(home, item.Dir, item.Dir)
	if err != nil {
		return nil, err
	}
	repo, err := openAuthority(abs, item)
	if err != nil {
		return nil, err
	}
	defer closeManagedSource(repo)
	published, err := writer.PublishSystem(repo)
	if err != nil {
		return nil, err
	}
	if err := stampAuthority(abs, item); err != nil {
		return nil, err
	}
	status := SystemRepositoryStatus(published.Commit)
	status["seeded"] = published.Seeded
	status["driver"] = item.Driver
	if item.DSN != "" {
		status["dsn"] = item.DSN
	}
	return status, nil
}

func systemAuthoritySpec(file HomeFile) (HomeRepo, bool) {
	for _, repo := range file.Repos {
		if repo.ID == string(knowledge.SystemRepositoryID) {
			return repo, true
		}
	}
	return HomeRepo{}, false
}
