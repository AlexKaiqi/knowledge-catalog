package cli

import (
	"fmt"
	"kc/invoke"
)

// Module declarations share presentation metadata. The five explicit owned
// assemblers remain the closed inventory; these helpers add no registrations,
// task parsing, business rules or execution dispatch.

type moduleCommandMetadata struct {
	Path, Handler, Action, Description, Usage string
	Flags, Operands, Groups                   []string
}
type moduleCommandDeclaration interface {
	~struct {
		Path, Handler, Action, Description, Usage string
		Flags, Operands, Groups                   []string
	}
}

func moduleCommandByHandler[T moduleCommandDeclaration](entries []T, handler string) (T, bool) {
	for _, entry := range entries {
		if moduleCommandMetadata(entry).Handler == handler {
			return entry, true
		}
	}
	var zero T
	return zero, false
}
func moduleCommandPath[T moduleCommandDeclaration](entries []T, path string) bool {
	for _, entry := range entries {
		if moduleCommandMetadata(entry).Path == path {
			return true
		}
	}
	return false
}
func bindModuleOperands(operands []string, flags map[string]FlagValue, args []string) error {
	if len(args) > len(operands) {
		return fmt.Errorf("unexpected argument %s", args[len(operands)])
	}
	for i, value := range args {
		name := operands[i]
		if invoke.FlagString(flags, name) != "" {
			return fmt.Errorf("unexpected argument %s", value)
		}
		flags[name] = value
	}
	return nil
}
func moduleHelpEntries[T moduleCommandDeclaration](entries []T) map[string]helpEntry {
	help := map[string]helpEntry{}
	for _, owned := range entries {
		entry := moduleCommandMetadata(owned)
		help[entry.Path] = helpEntry{Description: entry.Description, Usage: entry.Usage}
	}
	return help
}
func moduleHelpGroups[T moduleCommandDeclaration](entries []T, host map[string][]string) map[string][]string {
	for _, owned := range entries {
		entry := moduleCommandMetadata(owned)
		for _, group := range entry.Groups {
			host[group] = append(host[group], entry.Path)
		}
	}
	return host
}
func moduleValueHelp[T moduleCommandDeclaration](entries []T, host []string) []string {
	for _, entry := range entries {
		host = append(host, moduleCommandMetadata(entry).Path)
	}
	return host
}
func composeModuleCommands[T moduleCommandDeclaration](entries []T, host map[string]commandSurface) map[string]commandSurface {
	for _, owned := range entries {
		entry := moduleCommandMetadata(owned)
		if _, duplicate := host[entry.Path]; duplicate {
			panic("duplicate CLI command " + entry.Path)
		}
		host[entry.Path] = commandSurface{Handler: entry.Handler, Action: entry.Action}
	}
	return host
}
