package invocation

import (
	"fmt"
	"kc/identity"
	"kc/invoke"
	"sort"
	"strings"
)

// HTTPIdentity and HTTPAuthenticator retain the original invocation API.
// Request identity is shared with the independently usable Snapshot service.
type HTTPIdentity = identity.HTTPIdentity
type HTTPAuthenticator = identity.HTTPAuthenticator

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
