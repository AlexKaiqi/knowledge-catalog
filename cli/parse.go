package cli

import (
	"fmt"
	"strings"
)

type ParsedArgs struct {
	Command string
	Flags   map[string]FlagValue
	Args    []string
}

func setFlag(flags map[string]FlagValue, name string, value any) {
	existing, ok := flags[name]
	if !ok {
		flags[name] = value
		return
	}
	switch cur := existing.(type) {
	case []string:
		flags[name] = append(cur, fmt.Sprint(value))
	case bool:
		flags[name] = []string{fmt.Sprint(cur), fmt.Sprint(value)}
	default:
		flags[name] = []string{fmt.Sprint(cur), fmt.Sprint(value)}
	}
}

func isFlagToken(token string) bool {
	return token == "-h" || token == "-help" || strings.HasPrefix(token, "--")
}

func ParseArgs(argv []string) (ParsedArgs, error) {
	flags := map[string]FlagValue{}
	var command string
	var args []string
	for i := 0; i < len(argv); i++ {
		token := argv[i]
		if token == "--" {
			continue
		}
		if token == "" {
			continue
		}
		if token == "-h" || token == "-help" {
			setFlag(flags, "help", true)
			continue
		}
		if raw, ok := strings.CutPrefix(token, "--"); ok {
			if name, value, hasValue := strings.Cut(raw, "="); hasValue {
				setFlag(flags, name, value)
				continue
			}
			if i+1 < len(argv) && !isFlagToken(argv[i+1]) {
				setFlag(flags, raw, argv[i+1])
				i++
			} else {
				setFlag(flags, raw, true)
			}
			continue
		}
		if command != "" {
			args = append(args, token)
			continue
		}
		command = token
	}
	if command == "" {
		command = "help"
	}
	return ParsedArgs{Command: command, Flags: flags, Args: args}, nil
}
