// Package invoke holds the typed invocation contract shared by the CLI front
// and the embedded HTTP server: one result shape for every verb, plus the
// HTTP request/response plumbing both entry paths use. It sits below kc/cli
// and kc/cli/serve so neither side can cycle through the other.
package invoke

// RunResult is the single result shape returned by every verb invocation,
// whether it came from argv or from the HTTP facade.
type RunResult struct {
	Status int
	Stdout string
}
