package cli

import (
	"os"
	"path/filepath"

	apphome "kc/home"
	"kc/identity"
	"kc/kernel"
)

// ensureManagedRepositoryGrant applies a deployment-approved creation policy
// exactly once. The HTTP application write lock serializes this with normal
// grant/revoke operations. The receipt and rule share one durable policy write;
// a crash after this write cannot make a retry undo an intervening revocation.
func ensureManagedRepositoryGrant(dir string, g apphome.ManagedRepositoryGrant) error {
	if g.AllocationID == "" || g.Principal == "" || g.RepositoryID == "" || len(g.Actions) == 0 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "managed repository initial policy is incomplete")
	}
	if info, err := os.Stat(allowPath(dir)); err != nil || !info.Mode().IsRegular() {
		return kernel.Fail(kernel.ErrPreconditionFailed, "durable authorization policy is unavailable")
	}
	policy, err := ReadAllow(dir)
	if err != nil {
		return err
	}
	digest := kernel.CanonicalDigest(g)
	if previous, ok := policy.InitialGrants[g.AllocationID]; ok {
		if previous != digest {
			return kernel.Fail(kernel.ErrIdempotencyConflict, "managed repository initial policy does not match its allocation")
		}
		return nil
	}
	principal, err := identity.ResolvePrincipal(filepath.Join(dir, identity.Filename), g.Principal)
	if err != nil {
		return err
	}
	if policy.InitialGrants == nil {
		policy.InitialGrants = map[string]kernel.Digest{}
	}
	policy.InitialGrants[g.AllocationID] = digest
	policy.Rules = append(policy.Rules, AllowRule{
		ID:        "managed_" + g.AllocationID,
		Principal: principal,
		Repo:      g.RepositoryID,
		Actions:   append([]string(nil), g.Actions...),
	})
	return WriteAllow(dir, policy)
}
