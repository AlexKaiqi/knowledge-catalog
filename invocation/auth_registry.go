package invocation

import (
	"context"
	"fmt"
	"kc/identity"
	"kc/invoke"
	"net/http"
	"sort"
	"strings"
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
	User *identity.VerifiedUser
}

type HTTPAuthenticator interface {
	Name() string
	Authenticate(context.Context, http.Header) (HTTPIdentity, error)
}

// AuthenticatorFactory creates an HTTPAuthenticator from serve flags.
// Each factory validates its own required flags and returns an error when
// mandatory configuration is missing or malformed.
type AuthenticatorFactory func(flags map[string]invoke.FlagValue) (HTTPAuthenticator, error)

var authenticatorFactories = map[string]AuthenticatorFactory{}

// RegisterAuthenticator registers a named authenticator factory. The name
// becomes the value accepted by --auth. Registration is append-only; calling
// RegisterAuthenticator twice with the same name panics.
func RegisterAuthenticator(name string, factory AuthenticatorFactory) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		panic("auth: empty authenticator name")
	}
	if _, dup := authenticatorFactories[name]; dup {
		panic(fmt.Sprintf("auth: duplicate authenticator %q", name))
	}
	authenticatorFactories[name] = factory
}

// AvailableAuthModes returns the sorted list of registered authenticator names
// for use in help text and error messages. (CAP-ORG-01 B6.)
func AvailableAuthModes() []string {
	names := make([]string, 0, len(authenticatorFactories))
	for name := range authenticatorFactories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// LookupAuthenticator returns the registered factory for one --auth name.
func LookupAuthenticator(name string) (AuthenticatorFactory, bool) {
	f, ok := authenticatorFactories[name]
	return f, ok
}

// AuthenticatorNames lists registered authenticator names for help text.
func AuthenticatorNames() []string {
	names := make([]string, 0, len(authenticatorFactories))
	for name := range authenticatorFactories {
		names = append(names, name)
	}
	return names
}
