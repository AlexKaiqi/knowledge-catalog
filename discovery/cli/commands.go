package cli

import "strings"

var commandEntries = []Command{
	{Path: "search", Handler: "object-search", Action: "file.read", Flags: words("dataset revision query match match-mode eq neq in exists missing prefix contains gt gte lt lte sort limit continuation recall"), Groups: []string{"object"}},
	{Path: "relations", Handler: "object-relations", Action: "file.read", Flags: words("dataset revision object relation-type role direction limit continuation"), Groups: []string{"object"}},
	{Path: "traverse", Handler: "object-traverse", Action: "file.read", Flags: words("dataset revision object relation-type role direction max-hops min-hops limit continuation"), Groups: []string{"object"}},
	{Path: "operations access-spec describe", Handler: "operations-access-spec-describe", Action: "file.read", Flags: words("dataset revision"), Groups: []string{"object", "operations"}},
	{Path: "plugin", Handler: "object-plugin-help", Action: "inventory.read", Flags: words("name task repo commit ref file"), Operands: words("name task"), Groups: []string{"object"}},
}

func words(raw string) []string { return strings.Fields(raw) }
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
