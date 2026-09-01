package cli

import (
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/writer"
	"kc/snapshot"
)

// ensureSystemRepository is host bootstrap orchestration. Catalog Core sees
// only a Repository ID; the application root mounts the immutable protocol
// publication and verifies it against the binary trust root.
func ensureSystemRepository(home, catalogID string) (kernel.CommitID, error) {
	ws, err := Open(home)
	if err != nil {
		return "", err
	}
	defer ws.Close()

	repo, exists := ws.Store.Get(knowledge.SystemRepositoryID)
	if !exists {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "built-in System Repository is not mounted")
	}
	cat, _, err := ws.UseCatalog(catalogID)
	if err != nil {
		return "", err
	}
	if err := cat.RegisterRepository(knowledge.SystemRepositoryID); err != nil {
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
	for _, operation := range knowledge.SystemSchemaOperations() {
		current, readErr := knowledgeRepo.Read(operation.Address.ObjectID, head)
		if readErr != nil {
			return "", readErr
		}
		if got, want := kernel.CanonicalDigest(current.Value), kernel.CanonicalDigest(operation.Value); got != want {
			return "", kernel.Fail(kernel.ErrPreconditionFailed,
				"system schema %s digest %s does not match built-in trust root %s",
				operation.Address.ObjectID, got, want)
		}
	}
	return head, nil
}

func authorizeSystemRepository(action, repositoryID, principal string) (bool, error) {
	if repositoryID != string(knowledge.SystemRepositoryID) || principal == "" {
		return false, nil
	}
	if action == "file.read" || action == "projection.read" || action == "knowledge.access.describe" ||
		action == "workspace.resolve" || action == "workspace.consume" || strings.HasPrefix(action, "knowledge.") {
		return true, nil
	}
	return true, kernel.Fail(kernel.ErrForbidden, "%s cannot mutate System Repository %s", principal, repositoryID)
}

func systemRepositoryStatus(commit kernel.CommitID) map[string]any {
	return map[string]any{
		"repositoryId":     knowledge.SystemRepositoryID,
		"commit":           commit,
		"metaSchema":       knowledge.MetaSchemaV1,
		"metaSchemaDigest": knowledge.SystemMetaSchemaDigest(),
	}
}

func publishSystemRepository(home, driver, dsn, dir string) (map[string]any, error) {
	driver = strings.TrimSpace(driver)
	if driver == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "system publish requires --driver dolt or gitea")
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
		// Reuse DSN only. Home-relative Dir must go through resolveStoreDir(home),
		// not absStoreDir from the current working directory.
	}
	if driver == "gitea" && spec.DSN == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "gitea system publish requires --dsn http(s)://host/owner/name")
	}
	item, err := authority.prepare(stores, spec)
	if err != nil {
		return nil, err
	}
	item.ID = string(knowledge.SystemRepositoryID)
	abs, err := resolveStoreDir(home, item.Dir, item.Dir)
	if err != nil {
		return nil, err
	}
	repo, err := openAuthority(abs, item)
	if err != nil {
		return nil, err
	}
	published, err := writer.PublishSystem(repo)
	if err != nil {
		return nil, err
	}
	if err := stampAuthority(abs, item); err != nil {
		return nil, err
	}
	status := systemRepositoryStatus(published.Commit)
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
