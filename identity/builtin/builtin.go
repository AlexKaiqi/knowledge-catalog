// Package builtin registers the authenticators shipped with kc. Assembly roots
// import it for its side effect; nothing else names a concrete authenticator.
package builtin

import (
	_ "kc/identity/local"
	_ "kc/identity/taihu"
)
