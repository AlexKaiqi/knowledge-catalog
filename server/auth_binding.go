package server

import (
	"kc/identity"
	inv "kc/invocation"
	"path/filepath"
)

// The initialized identity authority is shared with independent file services;
// the facade selects its existing state path and controlled-fixture mode only.
func bindHTTPAuthenticator(base inv.HTTPAuthenticator, home string, fixture bool) inv.HTTPAuthenticator {
	return identity.WrapAuthenticator(base, filepath.Join(home, identity.Filename), fixture)
}
