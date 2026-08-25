package knowledge

import "kc/kernel"

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

func SelectAspects(value any, units []Address, selector *AspectSelector) any {
	if selector == nil {
		return value
	}
	hasAspect := false
	for _, unit := range units {
		if unit.AspectName != "" {
			hasAspect = true
			break
		}
	}
	if !hasAspect {
		return value
	}
	object, ok := value.(map[string]any)
	if !ok {
		return value
	}
	exclude := map[string]struct{}{}
	for _, name := range selector.Exclude {
		exclude[name] = struct{}{}
	}
	out := map[string]any{}
	for name, item := range object {
		if len(selector.Include) > 0 {
			found := false
			for _, include := range selector.Include {
				if include == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if _, skip := exclude[name]; skip {
			continue
		}
		out[name] = item
	}
	return out
}

type Resolution struct {
	Repository        kernel.RepositoryID `json:"repository"`
	Commit            kernel.CommitID     `json:"commit"`
	ObjectID          ObjectID            `json:"objectId"`
	Address           Address             `json:"address"`
	PathHint          string              `json:"pathHint"`
	Digest            kernel.Digest       `json:"digest,omitempty"`
	DeclarationDigest kernel.Digest       `json:"declarationDigest,omitempty"`
	SchemaRef         string              `json:"schemaRef,omitempty"`
	ValueSource       *ValueSource        `json:"valueSource,omitempty"`
	Status            ResolutionStatus    `json:"status"`
}

type KnowledgeValue struct {
	KnowledgeRef KnowledgeRef        `json:"knowledgeRef"`
	Repository   kernel.RepositoryID `json:"repository"`
	Commit       kernel.CommitID     `json:"commit"`
	Address      Address             `json:"address"`
	Value        any                 `json:"value"`
	Provenance   *ProvenanceEnvelope `json:"provenance,omitempty"`
	Units        []Address           `json:"units,omitempty"`
	Declarations []UnitDeclaration   `json:"declarations,omitempty"`
}

type ProvenanceTrace struct {
	Repository kernel.RepositoryID  `json:"repository"`
	Commit     kernel.CommitID      `json:"commit"`
	ObjectID   ObjectID             `json:"objectId"`
	Chain      []ProvenanceEnvelope `json:"chain"`
}
