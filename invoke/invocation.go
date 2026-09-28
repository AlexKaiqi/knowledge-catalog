package invoke

import (
	"context"
	"fmt"

	"kc/controlplane"
	"kc/home"
	"kc/kernel"
	knowledge "kc/knowledge"
	knowledgeserving "kc/knowledge/serving"
	"kc/snapshot"
)

// Stage is how much of the local workspace a verb needs before it runs. It is
// the only axis on which verbs differ before their own body starts, so it is
// declared per verb instead of re-derived by an if-chain in the dispatcher.
// (moved from cli/command.go, QUALITY-05 S3')
type Stage int

const (
	// StageHome resolves --home and nothing else. The verb opens what it needs,
	// which is how `audit` and `store-ls` still answer when Repository
	// attachment fails.
	StageHome Stage = iota

	// StageOpen opens this --home's object graph (stores, catalogs, writer)
	// and records the call. Used by the verbs that shape the home itself,
	// before any grant exists.
	StageOpen

	// StageGoverned mounts the workspace, checks allow.json, binds the control
	// plane and wraps the body in pre/post hooks. Every protocol verb is here.
	StageGoverned
)

// Invocation is one resolved verb call. WS is nil at StageHome. The type and
// its readers are the transport-neutral execution contract: CLI verbs and
// typed HTTP handlers both run inside one Invocation, so both entry paths
// import it from here instead of reaching across packages. (QUALITY-05 S3')
type Invocation struct {
	Command     string
	Home        string
	Flags       map[string]FlagValue
	WS          *home.Home
	Context     context.Context
	State       knowledgeserving.StateLookup
	Observation *ObservationTelemetry
}

// Flag reads the flag's effective string value.
func (cx *Invocation) Flag(name string) string { return FlagString(cx.Flags, name) }

// FlagValues reads every repetition of the flag, in order.
func (cx *Invocation) FlagValues(name string) []string { return FlagStrings(cx.Flags, name) }

// Require reads the flag and fails with a usage error when it is absent.
func (cx *Invocation) Require(name string) (string, error) {
	return RequireFlag(cx.Flags, name)
}

// SetID reads --dataset (the consume verbs' id vocabulary).
func (cx *Invocation) SetID() (string, error) {
	return RequireDataset(cx.Flags)
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
func (cx *Invocation) BindingScope() BindingScope {
	return BindingScope{On: cx.Flag("on"), Repo: cx.Flag("repo"), Catalog: cx.Flag("catalog")}
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

// Handler is one verb body: it receives the resolved Invocation and returns
// either a result for the transport to render or an error carrying a kernel
// fault code.
type Handler func(cx *Invocation) (any, error)

// Command pairs a verb's Stage with its body. CLI verbs are declared in the
// front-end registry; typed HTTP handlers pass an explicit Command value and
// never consult either registry.
type Command struct {
	Stage Stage
	Run   Handler
}

// BindingScope is the selector triple shared by the policy verbs.
type BindingScope struct {
	On      string
	Repo    string
	Catalog string
}

// Matches reports whether the scope selects the given coordinate: an empty
// selector matches everything.
func (s BindingScope) Matches(on, repo, catalogID string) bool {
	if s.On != "" && s.On != on {
		return false
	}
	if s.Repo != "" && s.Repo != repo {
		return false
	}
	if s.Catalog != "" && s.Catalog != catalogID {
		return false
	}
	return true
}
