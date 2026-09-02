package cli

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// HTTPAuthenticator proves one request's caller identity. Authorization
// policy remains in allow.json; authenticators only verify transport claims.
type HTTPAuthenticator interface {
	Name() string
	Authenticate(context.Context, http.Header) (HTTPIdentity, error)
}

// AuthenticatorFactory creates an HTTPAuthenticator from serve flags.
// Each factory validates its own required flags and returns an error when
// mandatory configuration is missing or malformed.
type AuthenticatorFactory func(flags map[string]FlagValue) (HTTPAuthenticator, error)

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
// for use in help text and error messages. "local" is a pairing mode, not a
// registered authenticator; see serveAuthModes.
func AvailableAuthModes() []string {
	names := make([]string, 0, len(authenticatorFactories))
	for name := range authenticatorFactories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func serveAuthModes() []string {
	seen := map[string]struct{}{"local": {}}
	for _, name := range AvailableAuthModes() {
		seen[name] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resolveAuthenticator looks up the factory for the given mode and calls it.
// It returns a clear error message when the mode is unknown.
func resolveAuthenticator(mode string, flags map[string]FlagValue) (HTTPAuthenticator, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	factory, ok := authenticatorFactories[mode]
	if !ok {
		all := AvailableAuthModes()
		if len(all) == 0 {
			return nil, fmt.Errorf("--auth %q: no authenticators registered", mode)
		}
		return nil, fmt.Errorf("kc serve requires --auth %s", strings.Join(serveAuthModes(), ", "))
	}
	authenticator, err := factory(flags)
	if err != nil {
		return nil, fmt.Errorf("--auth %s: %v", mode, err)
	}
	return authenticator, nil
}

// RegisterGiteaAuthenticator is defined in auth_gitea.go.
// RegisterTaihuAuthenticator is defined in auth_taihu.go.
