package cli

import (
	"kc/home"
)

// Home assembly lives in kc/home. The B6 verb-body move took the verbs to
// kc/invocation, so this file only keeps the store-flag glue the remaining
// cli-side verbs share; the former re-export aliases are gone (B8 closing
// sweep: callers reference kc/home directly).

func applyStoreFlags(file home.StoresFile, flags map[string]FlagValue) (home.StoresFile, error) {
	return home.ApplyStoreFlags(file, stringFlags(flags))
}

func stringFlags(flags map[string]FlagValue) map[string]string {
	out := map[string]string{}
	for name := range flags {
		out[name] = flagString(flags, name)
	}
	return out
}
