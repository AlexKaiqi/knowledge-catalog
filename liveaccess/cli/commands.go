package cli

import "strings"

var commandEntries = []Command{
	{Path: "binding show", Handler: "object-binding-show", Action: "object.binding.resolve", Flags: strings.Fields("repo dataset commit ref object aspect member"), Groups: []string{"object"}},
	{Path: "access", Handler: "object-access", Action: "resource.access", Flags: strings.Fields("repo dataset commit ref object aspect member"), Groups: []string{"object"}},
	{Path: "invoke", Handler: "object-invoke", Action: "resource.access", Flags: strings.Fields("repo dataset commit ref object implementation operation input"), Groups: []string{"object"}},
}

func Commands() []Command {
	entries := make([]Command, len(commandEntries))
	for i, entry := range commandEntries {
		help := leafHelpEntries()[entry.Path]
		entry.Description, entry.Usage = help.Description, help.Usage
		entry.Flags = append([]string(nil), entry.Flags...)
		entry.Operands = append([]string(nil), entry.Operands...)
		entry.Groups = append([]string(nil), entry.Groups...)
		entries[i] = entry
	}
	return entries
}

func command(path string) (Command, bool) {
	for _, entry := range commandEntries {
		if entry.Path == path {
			return entry, true
		}
	}
	return Command{}, false
}
