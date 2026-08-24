package cli

import (
	"fmt"
	"strings"
)

// FlagValue is a parsed --flag: string, bool for a bare flag, or []string when
// the flag repeats. Verbs read it through FlagString / FlagStrings / FlagBool so
// they never care which shape arrived.
type FlagValue any

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
		if raw, ok := strings.CutPrefix(token, "--"); ok {
			if name, value, hasValue := strings.Cut(raw, "="); hasValue {
				setFlag(flags, name, value)
				continue
			}
			if i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "--") {
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

func FlagString(flags map[string]FlagValue, name string) string {
	value, ok := flags[name]
	if !ok {
		return ""
	}
	switch v := value.(type) {
	case bool:
		return ""
	case []string:
		if len(v) == 0 {
			return ""
		}
		return v[len(v)-1]
	default:
		return fmt.Sprint(v)
	}
}

func FlagStrings(flags map[string]FlagValue, name string) []string {
	value, ok := flags[name]
	if !ok {
		return nil
	}
	switch v := value.(type) {
	case bool:
		return nil
	case []string:
		return v
	default:
		return []string{fmt.Sprint(v)}
	}
}

func FlagBool(flags map[string]FlagValue, name string) bool {
	value, ok := flags[name]
	if !ok {
		return false
	}
	b, ok := value.(bool)
	return ok && b
}

func RequireFlag(flags map[string]FlagValue, name string) (string, error) {
	value := FlagString(flags, name)
	if value == "" {
		return "", fmt.Errorf("missing --%s", name)
	}
	return value, nil
}
