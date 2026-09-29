package cli

import (
	"fmt"
)

// Pin, Serving and consumer authorization shared by workspace read/search.
// Product file entry remains Workspace File Gateway / kcfs.

func rejectRemovedFlags(flags map[string]FlagValue) error {
	for _, name := range []string{"view", "release", "generation", "base-generation", "input-vrv", "session-id"} {
		if _, exists := flags[name]; exists {
			return fmt.Errorf("unknown flag --%s", name)
		}
	}
	return nil
}

// openCompleteServing is for consumer operations whose public response has no
// coverage envelope (READ/RESOLVE/RELATIONS/LOG/PROVENANCE and similar exact reads). Those
// operations must not silently turn an authorization gap into an empty or
// apparently complete result. SEARCH keeps every pin member in the candidate
// set and applies the delivery chain after hydrate.
// requireCompleteWorkspaceRead fails closed when a bare-result consumer
// operation cannot see every member at the requested object scope. The error is
// intentionally generic: callers learn that the Workspace read is incomplete,
// not which hidden repository may contain the object.
