package cli

import "fmt"

type FlagValue any

type ParsedArgs struct {
	Command string
	Flags   map[string]FlagValue
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
	for i := 0; i < len(argv); i++ {
		token := argv[i]
		if token == "--" {
			continue
		}
		if token == "" {
			continue
		}
		if len(token) >= 2 && token[:2] == "--" {
			raw := token[2:]
			if eq := indexByte(raw, '='); eq >= 0 {
				setFlag(flags, raw[:eq], raw[eq+1:])
				continue
			}
			if i+1 < len(argv) && !hasPrefix(argv[i+1], "--") {
				setFlag(flags, raw, argv[i+1])
				i++
			} else {
				setFlag(flags, raw, true)
			}
			continue
		}
		if command != "" {
			return ParsedArgs{}, fmt.Errorf("unexpected argument %s", token)
		}
		command = token
	}
	if command == "" {
		command = "help"
	}
	return ParsedArgs{Command: command, Flags: flags}, nil
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

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
