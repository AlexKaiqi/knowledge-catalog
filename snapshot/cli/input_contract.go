package cli

import "kc/kernel"

// ValidateFlags checks only the module's parameter contract. It never reads
// recipe/rule files, performs HTTP requests or requires semantic operands.
// Reserved module flags remain scoped when a host validates its own leaves.
func (e *executor) ValidateFlags(path string, flags map[string]any) error {
	if _, present := flags["metadata"]; present && path != "dataset define" {
		return kernel.Fail(kernel.ErrUsageInvalid, "unknown flag --metadata")
	}
	for _, scope := range [][2]string{{"offset", "files read"}, {"length", "files read"}, {"expected-file", "grant update"}} {
		if _, present := flags[scope[0]]; present && path != scope[1] {
			return kernel.Fail(kernel.ErrUsageInvalid, "--%s is only valid for kc %s", scope[0], scope[1])
		}
	}
	allowed := commandFlags(path)
	if allowed == nil {
		// Nil preserves the leaf's existing caller-owned vocabulary, including
		// repeated sources. Host-owned flags are outside this module contract.
		return nil
	}
	if err := rejectFlagsOutside(flags, allowed, "kc "+path); err != nil {
		return err
	}
	for name, value := range flags {
		if _, repeated := value.([]string); repeated {
			return kernel.Fail(kernel.ErrUsageInvalid, "--%s must be supplied once", name)
		}
		if _, missing := value.(bool); missing && name != "help" {
			return kernel.Fail(kernel.ErrUsageInvalid, "--%s requires a value", name)
		}
	}
	return nil
}
