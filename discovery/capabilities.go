package discovery

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// IndexManifest describes one instance's declarative retrieval vocabulary.
// It contains no endpoint, credential or raw engine configuration.
type IndexManifest struct {
	Name    string                 `json:"name"`
	Version string                 `json:"version"`
	Builds  []IndexBuildCapability `json:"builds"`
	Inputs  []IndexInputOption     `json:"inputs"`
	Limits  IndexManifestLimits    `json:"limits"`
	Closed  []string               `json:"notDeclarable"`
	Digest  kernel.Digest          `json:"digest"`
}

type IndexBuildCapability struct {
	UnavailableReason string             `json:"unavailableReason,omitempty"`
	Kind              string             `json:"kind"`
	Category          string             `json:"category"`
	Engine            string             `json:"engine"`
	InputTypes        []string           `json:"inputTypes"`
	Uses              []string           `json:"uses"`
	MaxInputs         int                `json:"maxInputs"`
	Options           []IndexBuildOption `json:"options"`
	Requires          []string           `json:"requires,omitempty"`
}

type IndexBuildOption struct {
	UnavailableReason string   `json:"unavailableReason,omitempty"`
	Name              string   `json:"name"`
	Type              string   `json:"type"`
	Values            []string `json:"values"`
	Rebuild           bool     `json:"rebuild"`
}

type IndexInputOption struct {
	UnavailableReason string   `json:"unavailableReason,omitempty"`
	Name              string   `json:"name"`
	Type              string   `json:"type"`
	Values            []string `json:"values,omitempty"`
	Rebuild           bool     `json:"rebuild"`
}

type IndexManifestLimits struct {
	MaxInputs int `json:"maxInputs"`
	MaxTokens int `json:"maxTokens"`
}

type IndexInstanceManifest struct {
	Plugin   string        `json:"plugin"`
	Instance string        `json:"instance"`
	Catalog  string        `json:"catalog"`
	Manifest IndexManifest `json:"manifest"`
}

// DeclarationDocument is caller-supplied publication material, never a path
// for the server to open. Path is only its relative diagnostic coordinate.
type DeclarationDocument struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type DeclarationTrialRequest struct {
	Repository kernel.RepositoryID   `json:"repository"`
	Commit     kernel.CommitID       `json:"commit,omitempty"`
	Ref        string                `json:"ref,omitempty"`
	Documents  []DeclarationDocument `json:"documents"`
}

type DeclarationTrialMember struct {
	Path     string              `json:"path"`
	Member   objectmodel.Address `json:"member"`
	Status   string              `json:"status"`
	Reason   string              `json:"reason,omitempty"`
	Code     string              `json:"code,omitempty"`
	Catalog  string              `json:"catalog,omitempty"`
	Instance string              `json:"instance,omitempty"`
	Rebuild  *bool               `json:"rebuild,omitempty"`
}

type DeclarationTrialResult struct {
	Repository kernel.RepositoryID      `json:"repository"`
	Commit     kernel.CommitID          `json:"commit"`
	Members    []DeclarationTrialMember `json:"members"`
}
