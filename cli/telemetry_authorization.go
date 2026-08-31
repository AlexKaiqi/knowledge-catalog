package cli

import "kc/kernel"

type authorizationObserver func(decision string)

// observeAuthorizationResult keeps telemetry policy outside authorization.
// authorize only exposes its actual final decision at this source boundary.
func observeAuthorizationResult(observe authorizationObserver, authErr *error) func() {
	return func() {
		switch kernel.CodeOf(*authErr) {
		case "":
			emitAuthorizationDecision(observe, "allow")
		case kernel.ErrForbidden, kernel.ErrUnauthenticated:
			emitAuthorizationDecision(observe, "deny")
		}
	}
}

func emitAuthorizationDecision(observe authorizationObserver, decision string) {
	if observe != nil {
		observe(decision)
	}
}
