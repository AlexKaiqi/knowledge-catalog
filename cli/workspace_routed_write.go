package cli

import "kc/kernel"

// authorizeRoutedWrite checks a Writer operation after a Workspace recipe has
// selected the real target Repository. It is not a VFS write capability.
func authorizeRoutedWrite(cx *invocation, target kernel.RepositoryID) error {
	scoped := make(map[string]FlagValue, len(cx.Flags)+1)
	for key, value := range cx.Flags {
		if key == "workspace" {
			continue
		}
		scoped[key] = value
	}
	scoped["repo"] = string(target)
	return authorize(cx.Home, "writer.commit", scoped, cx.Observation.authorization)
}
