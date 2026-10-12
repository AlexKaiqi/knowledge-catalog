package identity

import (
	"context"
	"net/http"
)

// HTTPAuthenticator proves one request's caller identity. Authorization
// policy remains in allow.json; authenticators only verify transport claims.
// HTTPIdentity is the authenticated caller of one HTTP request. Principal is
// the only field used by allow.json. OnBehalfOf, when present, must be a
// delegation claim verified by the authenticator; request headers cannot set
// it in authenticated mode. The remaining fields make whoami useful without
// turning provider claims into authorization rules.
// HTTPIdentity carries the verified caller identity produced by one
// authenticator. (CAP-ORG-01 B6.)
type HTTPIdentity struct {
	Principal  string
	OnBehalfOf string
	Provider   string
	Subject    string
	Login      string
	Admin      bool
	// User contains the verifier's human identity, including the immutable
	// external subject and issuer. It is bound durably before authorization and
	// never exposed as a replacement for the username principal.
	User *VerifiedUser
}

type HTTPAuthenticator interface {
	Name() string
	Authenticate(context.Context, http.Header) (HTTPIdentity, error)
}
