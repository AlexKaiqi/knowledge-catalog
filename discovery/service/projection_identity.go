package service

import "kc/kernel"

func projectionViewDigest(parts []any, projection any, repos map[kernel.RepositoryID]kernel.CommitID) kernel.Digest {
	if selected, ok := projection.(interface {
		SelectionDigest(map[kernel.RepositoryID]kernel.CommitID) kernel.Digest
	}); ok {
		if identity := selected.SelectionDigest(repos); identity != "" {
			parts = append(parts, identity)
		}
	}
	return kernel.CanonicalDigest(parts)
}

func projectionScopeDigest(scope any, projection any, repos map[kernel.RepositoryID]kernel.CommitID) kernel.Digest {
	if selected, ok := projection.(interface {
		SelectionDigest(map[kernel.RepositoryID]kernel.CommitID) kernel.Digest
	}); ok {
		if identity := selected.SelectionDigest(repos); identity != "" {
			return kernel.CanonicalDigest([]any{scope, identity})
		}
	}
	return kernel.CanonicalDigest(scope)
}
