package knowledge

import (
	"strings"

	"kc/kernel"
)

type RelationDirection string

const (
	RelationDirected   RelationDirection = "DIRECTED"
	RelationUndirected RelationDirection = "UNDIRECTED"
)

// RelationEndpoint is one typed role in a Canonical Relation. ObjectRef is a
// repository-qualified, path-independent KnowledgeRef.
type RelationEndpoint struct {
	Role      string       `json:"role"`
	ObjectRef KnowledgeRef `json:"objectRef"`
}

// CanonicalRelation is the common envelope shared by domain relation types.
// Domain-specific facts stay in Attributes and are constrained by schema_ref.
type CanonicalRelation struct {
	RelationID   ObjectID           `json:"relationId"`
	RelationType string             `json:"relationType"`
	Direction    RelationDirection  `json:"direction"`
	Endpoints    []RelationEndpoint `json:"endpoints"`
	Attributes   map[string]any     `json:"attributes,omitempty"`
	Validity     map[string]any     `json:"validity,omitempty"`
}

// DecodeRelation validates the common Relation envelope. Endpoint existence is
// a Workspace-level concern and is deliberately not checked during one-Repo
// COMMIT.
func DecodeRelation(address Address, value any) (CanonicalRelation, error) {
	if address.Kind != KindRelation {
		return CanonicalRelation{}, kernel.Fail(kernel.ErrUsageInvalid, "address %s is not a Relation", AddressKey(address))
	}
	body, ok := value.(map[string]any)
	if !ok {
		return CanonicalRelation{}, kernel.Fail(kernel.ErrUsageInvalid, "relation %s must be a JSON object", address.ObjectID)
	}
	relationIDText, ok := relationField(body, "relationId")
	relationID := ObjectID(relationIDText)
	if !ok || relationID == "" || relationID != address.ObjectID {
		return CanonicalRelation{}, kernel.Fail(kernel.ErrUsageInvalid, "relation %s must declare the same relationId", address.ObjectID)
	}
	relationType, ok := relationField(body, "relationType")
	if !ok || relationType == "" {
		return CanonicalRelation{}, kernel.Fail(kernel.ErrUsageInvalid, "relation %s requires relationType", address.ObjectID)
	}
	directionText, ok := relationField(body, "direction")
	direction := RelationDirection(directionText)
	if !ok || (direction != RelationDirected && direction != RelationUndirected) {
		return CanonicalRelation{}, kernel.Fail(kernel.ErrUsageInvalid, "relation %s direction must be DIRECTED or UNDIRECTED", address.ObjectID)
	}
	rawEndpoints, ok := body["endpoints"].([]any)
	if !ok || len(rawEndpoints) < 2 {
		return CanonicalRelation{}, kernel.Fail(kernel.ErrUsageInvalid, "relation %s requires at least two endpoints", address.ObjectID)
	}
	endpoints := make([]RelationEndpoint, 0, len(rawEndpoints))
	seen := map[string]struct{}{}
	for i, raw := range rawEndpoints {
		item, ok := raw.(map[string]any)
		if !ok {
			return CanonicalRelation{}, kernel.Fail(kernel.ErrUsageInvalid, "relation %s endpoint %d must be an object", address.ObjectID, i)
		}
		role, roleOK := relationField(item, "role")
		objectRef, refOK := relationKnowledgeRef(item["objectRef"])
		endpoint := RelationEndpoint{Role: role, ObjectRef: objectRef}
		if !roleOK || !refOK || endpoint.Role == "" {
			return CanonicalRelation{}, kernel.Fail(kernel.ErrUsageInvalid, "relation %s endpoint %d requires role and objectRef", address.ObjectID, i)
		}
		key := endpoint.Role + "\x00" + string(endpoint.ObjectRef.Repository) + "\x00" + string(endpoint.ObjectRef.Object)
		if _, duplicate := seen[key]; duplicate {
			return CanonicalRelation{}, kernel.Fail(kernel.ErrUsageInvalid, "relation %s repeats endpoint %s", address.ObjectID, key)
		}
		seen[key] = struct{}{}
		endpoints = append(endpoints, endpoint)
	}
	relation := CanonicalRelation{
		RelationID: relationID, RelationType: relationType, Direction: direction, Endpoints: endpoints,
	}
	if raw, exists := body["attributes"]; exists {
		attributes, ok := raw.(map[string]any)
		if !ok {
			return CanonicalRelation{}, kernel.Fail(kernel.ErrUsageInvalid, "relation %s attributes must be an object", address.ObjectID)
		}
		relation.Attributes = attributes
	}
	if raw, exists := body["validity"]; exists {
		validity, ok := raw.(map[string]any)
		if !ok {
			return CanonicalRelation{}, kernel.Fail(kernel.ErrUsageInvalid, "relation %s validity must be an object", address.ObjectID)
		}
		relation.Validity = validity
	}
	return relation, nil
}

func relationField(body map[string]any, field string) (string, bool) {
	text, ok := body[field].(string)
	return strings.TrimSpace(text), ok
}

func relationKnowledgeRef(raw any) (KnowledgeRef, bool) {
	body, ok := raw.(map[string]any)
	if !ok {
		return KnowledgeRef{}, false
	}
	repository, repositoryOK := relationField(body, "repository")
	object, objectOK := relationField(body, "object")
	if !repositoryOK || !objectOK || repository == "" || object == "" {
		return KnowledgeRef{}, false
	}
	return KnowledgeRef{Repository: kernel.RepositoryID(repository), Object: ObjectID(object)}, true
}
