package cli

import "strings"

var commandEntries = []Command{
	{Path: "operations projection describe", Handler: "operations-projection-describe", Action: "file.read", Flags: strings.Fields("dataset revision"), Groups: []string{"operations"}},
	{Path: "operations projection sync", Handler: "operations-projection-sync", Action: "dataset.manage", Flags: strings.Fields("dataset revision"), Groups: []string{"operations"}},
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

// Shape leaves the established maintenance result, including StateSync, intact.
func Shape(_ string, value any) any { return value }
