package cli

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

// invocation is one resolved verb call. WS is nil at StageHome. The struct and
// its readers are the execution contract shared by the CLI front end and the
// typed HTTP service surface: both run one Invocation per call. QUALITY-05 S3'
// moved it to kc/invoke; CAP-ORG-01 R1 moved it back — the struct carries the
// transport-opened *home.Home that 143 verb bodies consume, so it is a
// workspace carrier, not transport-neutral vocabulary. Only the transport
// (cli) opens Home; invoke stays a pure flag/JSON/observation vocabulary.
type invocation struct {
	Command     string
	Home        string
	Flags       map[string]FlagValue
	WS          *home.Home
	Context     context.Context
	State       knowledgeserving.StateLookup
	Observation *operationTelemetry
}

// Flag reads the flag's effective string value.
func (cx *invocation) Flag(name string) string { return FlagString(cx.Flags, name) }

// FlagValues reads every repetition of the flag, in order.
func (cx *invocation) FlagValues(name string) []string { return FlagStrings(cx.Flags, name) }

// Require reads the flag and fails with a usage error when it is absent.
func (cx *invocation) Require(name string) (string, error) {
	return RequireFlag(cx.Flags, name)
}

// SetID reads --dataset (the consume verbs' id vocabulary).
func (cx *invocation) SetID() (string, error) {
	return setIDFlag(cx.Flags)
}

// TargetRef resolves a --ref style flag against snapshot.DefaultRef.
func (cx *invocation) TargetRef(name string) string {
	return snapshot.RefOrDefault(cx.Flag(name))
}

// RequireProposal resolves the --proposal flag against the workspace control
// plane, failing with a usage error when the proposal is unknown.
func (cx *invocation) RequireProposal(flag string) (controlplane.Proposal, error) {
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
func (cx *invocation) RequirePreview(flag string) (controlplane.Preview, error) {
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
func (cx *invocation) BindingScope() invoke.BindingScope {
	return invoke.BindingScope{On: cx.Flag("on"), Repo: cx.Flag("repo"), Catalog: cx.Flag("catalog")}
}

// KnowledgeRef builds a knowledge ref from the --object flag under the given
// repository.
func (cx *invocation) KnowledgeRef(repositoryID kernel.RepositoryID) (knowledge.KnowledgeRef, error) {
	objectID, err := cx.Require("object")
	if err != nil {
		return knowledge.KnowledgeRef{}, err
	}
	return knowledge.KnowledgeRef{Repository: repositoryID, Object: knowledge.ObjectID(objectID)}, nil
}

// Handler is one verb body: it receives the resolved Invocation and returns
// either a result for the transport to render or an error carrying a kernel
// fault code.
type handler func(cx *invocation) (any, error)

// Command pairs a verb's Stage with its body. CLI verbs are declared in the
// front-end registry; typed HTTP handlers pass an explicit Command value and
// never consult either registry.
type command struct {
	Stage invoke.Stage
	Run   handler
}
