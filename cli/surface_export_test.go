package cli

import "sort"

// CLICommandsForTest exposes the single CLI surface only inside the Go test
// binary. Coverage evidence does not justify expanding the production Go API.
func CLICommandsForTest() []string {
	commands := make([]string, 0, len(cliSurface))
	for path := range cliSurface {
		commands = append(commands, path)
	}
	sort.Strings(commands)
	return commands
}

func CLICommandForTest(path string) bool {
	_, ok := cliSurface[path]
	return ok
}

func CLICommandActionsForTest() map[string]string {
	actions := make(map[string]string, len(cliSurface))
	for path, surface := range cliSurface {
		actions[path] = surface.Action
	}
	return actions
}
