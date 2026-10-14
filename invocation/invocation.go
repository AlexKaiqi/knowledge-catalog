// Package invocation is the shared execution seam that carries one resolved
// operation call: the workspace carrier, the command/handler pair, and their
// readers. Both transports (the CLI front end and, from CAP-ORG-01 B6, the
// service surface) run one Invocation per call; neither may own the type,
// because the two transports must never import each other. The seam mirrors
// internal/telemetry: shared application-layer infrastructure that is not a
// user-facing capability. It holds the transport-opened *home.Home that verb
// bodies consume — it never opens one itself — so it is deliberately not part
// of kc/invoke, which stays a pure flag/JSON/observation vocabulary
// (QUALITY-05 S3', CAP-ORG-01 R1 and B5a).
package invocation

import (
	"context"

	"kc/home"
	"kc/invoke"
	"kc/kernel"
	objectserving "kc/liveaccess"
	objectmodel "kc/object"
	"kc/snapshot"
)

// Invocation is one resolved operation call. WS is nil at StageHome. The
// struct and its readers are the execution contract shared by the CLI front
// end and the typed service surface.
type Invocation struct {
	Command     string
	Home        string
	Flags       map[string]invoke.FlagValue
	WS          *home.Home
	Context     context.Context
	State       objectserving.StateLookup
	Observation *invoke.ObservationTelemetry
}

// Flag reads the flag's effective string value.
func (cx *Invocation) Flag(name string) string { return invoke.FlagString(cx.Flags, name) }

// FlagValues reads every repetition of the flag, in order.
func (cx *Invocation) FlagValues(name string) []string { return invoke.FlagStrings(cx.Flags, name) }

// Require reads the flag and fails with a usage error when it is absent.
func (cx *Invocation) Require(name string) (string, error) {
	return invoke.RequireFlag(cx.Flags, name)
}

// SetID reads --dataset (the consume verbs' id vocabulary).
func (cx *Invocation) SetID() (string, error) {
	return invoke.RequireDataset(cx.Flags)
}

// TargetRef resolves a --ref style flag against snapshot.DefaultRef.
func (cx *Invocation) TargetRef(name string) string {
	return snapshot.RefOrDefault(cx.Flag(name))
}

// BindingScope groups the --on / --repo / registry selectors used by the
// policy verbs.
func (cx *Invocation) BindingScope() invoke.BindingScope {
	return invoke.BindingScope{On: cx.Flag("on"), Repo: cx.Flag("repo")}
}

// ObjectRef builds an object ref from the --object flag under the given
// repository.
func (cx *Invocation) ObjectRef(repositoryID kernel.RepositoryID) (objectmodel.ObjectRef, error) {
	objectID, err := cx.Require("object")
	if err != nil {
		return objectmodel.ObjectRef{}, err
	}
	return objectmodel.ObjectRef{Repository: repositoryID, Object: objectmodel.ObjectID(objectID)}, nil
}

// Handler is one operation body: it receives the resolved Invocation and
// returns either a result for the transport to render or an error carrying a
// kernel fault code.
type Handler func(cx *Invocation) (any, error)

// Command pairs an operation's Stage with its body. CLI operations are
// declared in the front-end registry; typed service handlers pass an explicit
// Command value and never consult either registry.
type Command struct {
	Stage invoke.Stage
	Run   Handler
}
