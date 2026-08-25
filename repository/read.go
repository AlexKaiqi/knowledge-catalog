package repository

import "kc/kernel"

// RESOLVE / READ on a pinned commit. AspectSelector filters assembled Aspect maps.

type ResolutionStatus string

const (
	StatusResolved   ResolutionStatus = "RESOLVED"
	StatusRemoved    ResolutionStatus = "REMOVED"
	StatusUnresolved ResolutionStatus = "UNRESOLVED"
	StatusForbidden  ResolutionStatus = "FORBIDDEN"
)

type AspectSelector struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

func SelectAspects(value any, units []kernel.Address, selector *AspectSelector) any {
	if selector == nil {
		return value
	}
	hasAspect := false
	for _, u := range units {
		if u.AspectName != "" {
			hasAspect = true
			break
		}
	}
	if !hasAspect {
		return value
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return value
	}
	exclude := map[string]struct{}{}
	for _, e := range selector.Exclude {
		exclude[e] = struct{}{}
	}
	out := map[string]any{}
	for k, v := range obj {
		if len(selector.Include) > 0 {
			found := false
			for _, inc := range selector.Include {
				if inc == k {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if _, skip := exclude[k]; skip {
			continue
		}
		out[k] = v
	}
	return out
}

type Resolution struct {
	Repository        kernel.RepositoryID `json:"repository"`
	Commit            kernel.CommitID     `json:"commit"`
	ObjectID          kernel.ObjectID     `json:"objectId"`
	Address           kernel.Address      `json:"address"`
	PathHint          string              `json:"pathHint"`
	Digest            kernel.Digest       `json:"digest,omitempty"`
	DeclarationDigest kernel.Digest       `json:"declarationDigest,omitempty"`
	SchemaRef         string              `json:"schemaRef,omitempty"`
	ValueSource       *ValueSource        `json:"valueSource,omitempty"`
	Status            ResolutionStatus    `json:"status"`
}

type KnowledgeValue struct {
	KnowledgeRef kernel.KnowledgeRef        `json:"knowledgeRef"`
	Repository   kernel.RepositoryID        `json:"repository"`
	Commit       kernel.CommitID            `json:"commit"`
	Address      kernel.Address             `json:"address"`
	Value        any                        `json:"value"`
	Provenance   *kernel.ProvenanceEnvelope `json:"provenance,omitempty"`
	Units        []kernel.Address           `json:"units,omitempty"`
	Declarations []UnitDeclaration          `json:"declarations,omitempty"`
}

type ProvenanceTrace struct {
	Repository kernel.RepositoryID         `json:"repository"`
	Commit     kernel.CommitID             `json:"commit"`
	ObjectID   kernel.ObjectID             `json:"objectId"`
	Chain      []kernel.ProvenanceEnvelope `json:"chain"`
}
