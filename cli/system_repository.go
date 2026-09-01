package cli

import (
	"strings"

	"kc/kernel"
	"kc/knowledge"
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
