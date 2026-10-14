package cli

import "kc/invoke"

// FlagValue preserves the public argv contract. Module tasks own their
// validation; the host supplies the parsed values and authenticated context.
type FlagValue = invoke.FlagValue
