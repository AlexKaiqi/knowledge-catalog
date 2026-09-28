package invocation

import (
	"kc/invoke"
	"kc/kernel"
)

// observeAuthorizationResult keeps telemetry policy outside authorization.
// authorize only exposes its actual final decision at this source boundary.
func observeAuthorizationResult(observe invoke.AuthorizationObserver, authErr *error) func() {
	return func() {
		switch kernel.CodeOf(*authErr) {
		case "":
			emitAuthorizationDecision(observe, "allow")
		case kernel.ErrForbidden, kernel.ErrUnauthenticated:
			emitAuthorizationDecision(observe, "deny")
		}
	}
}

func emitAuthorizationDecision(observe invoke.AuthorizationObserver, decision string) {
	if observe != nil {
		observe(decision)
	}
}
