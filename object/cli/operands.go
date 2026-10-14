package cli

// operandValue distinguishes a positional value bound by this module from a
// user-supplied flag. It never enters the typed HTTP request.
type operandValue string

// BindOperands binds the module's declared positionals. The host passes raw
// flags and operands without interpreting Object references.
func BindOperands(path string, flags map[string]any, args []string) error {
	entry, found := command(path)
	if !found {
		return usage("Object CLI does not implement %s", path)
	}
	if len(args) > len(entry.Operands) {
		return usage("unexpected argument %s", args[len(entry.Operands)])
	}
	for i, value := range args {
		name := entry.Operands[i]
		if _, supplied := flags[name]; supplied {
			return usage("kc %s rejects --%s; supply the object URN once as its positional argument", path, name)
		}
		flags[name] = operandValue(value)
	}
	return nil
}
