package server

import (
	"fmt"
	inv "kc/invocation"
	"sort"
	"strings"
)

func serveAuthModes() []string {
	seen := map[string]struct{}{"local": {}}
	for _, name := range inv.AvailableAuthModes() {
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
func resolveAuthenticator(mode string, flags map[string]FlagValue) (inv.HTTPAuthenticator, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	factory, ok := inv.LookupAuthenticator(mode)
	if !ok {
		all := inv.AvailableAuthModes()
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
