package snapshot

import "kc/snapshot/internal/storecontract"

// RejectConfiguredSecret refuses passwords or API keys in connection text.
func RejectConfiguredSecret(kind, raw, env string) error {
	return storecontract.RejectConfiguredSecret(kind, raw, env)
}
