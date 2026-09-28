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
	"fmt"

	"kc/controlplane"
	"kc/home"
	"kc/invoke"
	"kc/kernel"
	knowledge "kc/knowledge"
	knowledgeserving "kc/knowledge/serving"
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
	State       knowledgeserving.StateLookup
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

// RequireProposal resolves the --proposal flag against the workspace control
// plane, failing with a usage error when the proposal is unknown.
func (cx *Invocation) RequireProposal(flag string) (controlplane.Proposal, error) {
	id, err := cx.Require(flag)
	if err != nil {
		return controlplane.Proposal{}, err
	}
	proposal, ok := cx.WS.Control.Proposals[id]
	if !ok {
		return controlplane.Proposal{}, fmt.Errorf("unknown proposal; run propose first")
	}
	return proposal, nil
}

// RequirePreview resolves the --preview flag against the workspace control
// plane, failing with a usage error when the preview is unknown.
func (cx *Invocation) RequirePreview(flag string) (controlplane.Preview, error) {
	id, err := cx.Require(flag)
	if err != nil {
		return controlplane.Preview{}, err
	}
	preview, ok := cx.WS.Control.Previews[id]
	if !ok {
		return controlplane.Preview{}, fmt.Errorf("unknown preview; run preview first")
	}
	return preview, nil
}

// BindingScope groups the --on / --repo / --catalog selectors used by the
// policy verbs.
func (cx *Invocation) BindingScope() invoke.BindingScope {
	return invoke.BindingScope{On: cx.Flag("on"), Repo: cx.Flag("repo"), Catalog: cx.Flag("catalog")}
}

// KnowledgeRef builds a knowledge ref from the --object flag under the given
// repository.
func (cx *Invocation) KnowledgeRef(repositoryID kernel.RepositoryID) (knowledge.KnowledgeRef, error) {
	objectID, err := cx.Require("object")
	if err != nil {
		return knowledge.KnowledgeRef{}, err
	}
	return knowledge.KnowledgeRef{Repository: repositoryID, Object: knowledge.ObjectID(objectID)}, nil
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
