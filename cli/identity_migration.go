package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"kc/identity"
	"kc/kernel"
)

// IdentityMigrationRequest is an explicit operator decision for an old
// provider-prefixed principal. The operator verifies the issuer and immutable
// subject independently; matching usernames alone never authorizes migration.
type IdentityMigrationRequest struct {
	LegacyPrincipal string                `json:"legacyPrincipal"`
	User            identity.VerifiedUser `json:"user"`
}

type IdentityMigrationReceipt struct {
	Digest        kernel.Digest `json:"digest"`
	MigratedRules int           `json:"migratedRules"`
}

type IdentityMigrationResult struct {
	Status        string `json:"status"`
	Principal     string `json:"principal"`
	MigratedRules int    `json:"migratedRules"`
}

var identityMigrationLocks sync.Map

// MigrateLegacyIdentity moves only extant rules and atomically stores a receipt
// in the same policy file. It never reapplies a creation policy, and retries
// cannot recreate revoked rules. Deployment management calls it while the
// service is stopped; embedded callers must serialize it with grant mutations.
func MigrateLegacyIdentity(stateDir string, request IdentityMigrationRequest) (IdentityMigrationResult, error) {
	if _, err := identity.CanonicalUsername(request.User.Username); err != nil {
		return IdentityMigrationResult{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid migration username: %v", err)
	}
	expected := ""
	switch request.User.Provider {
	case "gitea":
		subject, err := strconv.ParseInt(request.User.Subject, 10, 64)
		if err != nil || subject <= 0 || strconv.FormatInt(subject, 10) != request.User.Subject {
			return IdentityMigrationResult{}, kernel.Fail(kernel.ErrUsageInvalid, "Gitea migration requires a positive stable numeric subject")
		}
		expected = "gitea:" + request.User.Subject
	case "taihu":
		expected = "taihu:" + request.User.Username
	default:
		return IdentityMigrationResult{}, kernel.Fail(kernel.ErrUsageInvalid, "legacy migration supports only Gitea and Taihu user identities")
	}
	if request.LegacyPrincipal != expected {
		return IdentityMigrationResult{}, kernel.Fail(kernel.ErrUsageInvalid, "legacy principal does not match the explicitly selected provider identity")
	}
	if request.User.Issuer == "" || request.User.Subject == "" {
		return IdentityMigrationResult{}, kernel.Fail(kernel.ErrUsageInvalid, "migration requires a trusted issuer and immutable subject")
	}
	absolute, err := filepath.Abs(stateDir)
	if err != nil {
		return IdentityMigrationResult{}, err
	}
	lock, _ := identityMigrationLocks.LoadOrStore(absolute, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	if _, err := os.Stat(allowPath(absolute)); err != nil {
		return IdentityMigrationResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "durable authorization state is unavailable")
	}
	policy, err := ReadAllow(absolute)
	if err != nil {
		return IdentityMigrationResult{}, err
	}
	digest := kernel.CanonicalDigest(request)
	previous, replayed := policy.IdentityMigrations[request.LegacyPrincipal]
	if replayed && previous.Digest != digest {
		return IdentityMigrationResult{}, kernel.Fail(kernel.ErrIdempotencyConflict, "legacy identity was already migrated using another explicit binding")
	}
	if err := identity.MigrateLegacyAlias(filepath.Join(absolute, identity.Filename), request.LegacyPrincipal, request.User); err != nil {
		return IdentityMigrationResult{}, err
	}
	if replayed {
		return IdentityMigrationResult{Status: "REPLAYED", Principal: request.User.Username, MigratedRules: previous.MigratedRules}, nil
	}
	migrated := 0
	for i := range policy.Rules {
		if policy.Rules[i].Principal == request.LegacyPrincipal {
			policy.Rules[i].Principal = request.User.Username
			migrated++
		}
	}
	if policy.IdentityMigrations == nil {
		policy.IdentityMigrations = map[string]IdentityMigrationReceipt{}
	}
	policy.IdentityMigrations[request.LegacyPrincipal] = IdentityMigrationReceipt{Digest: digest, MigratedRules: migrated}
	if err := WriteAllow(absolute, policy); err != nil {
		return IdentityMigrationResult{}, err
	}
	return IdentityMigrationResult{Status: "APPLIED", Principal: request.User.Username, MigratedRules: migrated}, nil
}

func readIdentityMigrationRequest(path string) (IdentityMigrationRequest, error) {
	var request IdentityMigrationRequest
	file, err := os.Open(path)
	if err != nil {
		return request, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, kernel.Fail(kernel.ErrUsageInvalid, "invalid identity migration request: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return request, kernel.Fail(kernel.ErrUsageInvalid, "identity migration requires exactly one JSON request")
	}
	if strings.TrimSpace(request.LegacyPrincipal) == "" {
		return request, kernel.Fail(kernel.ErrUsageInvalid, "identity migration requires legacyPrincipal")
	}
	return request, nil
}
