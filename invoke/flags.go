package invoke

import "fmt"

// FlagValue is a parsed --flag: string, bool for a bare flag, or []string when
// the flag repeats. Verbs read it through FlagString / FlagStrings / FlagBool
// so they never care which shape arrived. The argv tokenizer (ParseArgs) stays
// front-end owned; this is the transport-neutral flag vocabulary that both the
// CLI and the HTTP surface read. (QUALITY-05 S3')
type FlagValue any

// FlagString returns the flag's effective value as a string. A bare flag or an
// empty repetition yields "".
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

// FlagStrings returns every repetition of the flag, in order.
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

// FlagBool reports whether the flag was present as a bare boolean.
func FlagBool(flags map[string]FlagValue, name string) bool {
	value, ok := flags[name]
	if !ok {
		return false
	}
	b, ok := value.(bool)
	return ok && b
}

// RequireFlag is FlagString with a usage error when the flag is absent.
func RequireFlag(flags map[string]FlagValue, name string) (string, error) {
	value := FlagString(flags, name)
	if value == "" {
		return "", fmt.Errorf("missing --%s", name)
	}
	return value, nil
}

// DatasetOf reads the --dataset flag (the id vocabulary of consume verbs).
func DatasetOf(flags map[string]FlagValue) string {
	return FlagString(flags, "dataset")
}

// RequireDataset is DatasetOf with a usage error when absent.
func RequireDataset(flags map[string]FlagValue) (string, error) {
	dataset := DatasetOf(flags)
	if dataset == "" {
		return "", fmt.Errorf("missing --dataset")
	}
	return dataset, nil
}
