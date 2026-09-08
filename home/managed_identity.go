package home

import (
	"path/filepath"

	"kc/identity"
	"kc/kernel"
)

func (ws *Home) managedPrincipal(principal string) (string, error) {
	return identity.ResolvePrincipal(filepath.Join(ws.Dir, identity.Filename), principal)
}

// managedRequestForOwner recovers a historical request only after an explicit
// operator alias connects its old actor to the current verified principal.
// All non-identity coordinates still need the original canonical digest.
func (ws *Home) managedRequestForOwner(req ManagedRepositoryRequest) (ManagedRepositoryRequest, error) {
	records, err := loadManagedRecords(ws.Dir)
	if err != nil {
		return req, err
	}
	for _, record := range records {
		if record.Request.RepositoryID != req.RepositoryID || record.Request.CommandID != req.CommandID || record.Request.Principal == req.Principal {
			continue
		}
		owner, err := ws.managedPrincipal(record.Request.Principal)
		if err != nil {
			return req, err
		}
		if owner != req.Principal {
			continue
		}
		original := req
		original.Principal = record.Request.Principal
		original.IdentityProvider, original.IdentityIssuer, original.IdentitySubject = record.Request.IdentityProvider, record.Request.IdentityIssuer, record.Request.IdentitySubject
		if kernel.CanonicalDigest(original) != record.Digest {
			return req, kernel.Fail(kernel.ErrIdempotencyConflict, "migrated owner retry changed the original managed request")
		}
		return original, nil
	}
	return req, nil
}

func (ws *Home) managedOwnerResult(record managedRecord) (ManagedRepositoryResult, error) {
	result := managedResult(record)
	var err error
	result.Owner, err = ws.managedPrincipal(result.Owner)
	return result, err
}
