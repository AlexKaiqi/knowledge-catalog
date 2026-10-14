package invocation

import "kc/home"

// VerbPluginHelp is an explanation task. StageHome keeps fixed reading and
// help available without mounting or invoking the described extension.
func VerbPluginHelp(cx *Invocation) (any, error) {
	return home.ExplainPlugin(cx.Flag("name"))
}
