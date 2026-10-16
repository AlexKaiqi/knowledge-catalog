package client

import (
	"encoding/json"
	retrieval "kc/discovery"
	objectmodel "kc/object"
	objectclient "kc/object/client"
)

type ObjectScope = objectclient.ObjectScope

// RerankRequest and SearchRerankRequest keep composition inputs with Discovery.
type RerankRequest struct {
	objectclient.ObjectDatasetIdentity
	Candidates []objectmodel.ObjectRef        `json:"candidates"`
	Spec       retrieval.SemanticOperatorSpec `json:"spec"`
}

type SearchRerankRequest struct {
	ObjectSearchRequest
	Spec retrieval.SemanticOperatorSpec `json:"spec"`
}

type ObjectSearchRequest = SearchRequest

type SearchRequest struct {
	ObjectScope
	Query        string                  `json:"query,omitempty"`
	Match        []string                `json:"match,omitempty"`
	MatchMode    string                  `json:"matchMode,omitempty"`
	Equal        []string                `json:"equal,omitempty"`
	NotEqual     []string                `json:"notEqual,omitempty"`
	In           []string                `json:"in,omitempty"`
	Exists       []string                `json:"exists,omitempty"`
	Missing      []string                `json:"missing,omitempty"`
	Prefix       []string                `json:"prefix,omitempty"`
	Contains     []string                `json:"contains,omitempty"`
	GreaterThan  []string                `json:"greaterThan,omitempty"`
	GreaterEqual []string                `json:"greaterEqual,omitempty"`
	LessThan     []string                `json:"lessThan,omitempty"`
	LessEqual    []string                `json:"lessEqual,omitempty"`
	Sort         []string                `json:"sort,omitempty"`
	Limit        int                     `json:"limit,omitempty"`
	Continuation string                  `json:"continuation,omitempty"`
	Expression   *retrieval.SearchExpr   `json:"expression,omitempty"`
	Order        *retrieval.SearchClause `json:"order,omitempty"`
	Recall       string                  `json:"recall,omitempty"`
}
type RelationsRequest struct {
	ObjectScope
	Endpoint     string `json:"endpoint"`
	RelationType string `json:"relationType,omitempty"`
	Role         string `json:"role,omitempty"`
	Direction    string `json:"direction,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}
type TraverseRequest struct {
	ObjectScope
	Endpoint     string `json:"endpoint"`
	RelationType string `json:"relationType,omitempty"`
	Role         string `json:"role,omitempty"`
	Direction    string `json:"direction,omitempty"`
	MinHops      int    `json:"minHops,omitempty"`
	MaxHops      int    `json:"maxHops"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

// AccessSpecDescribeRequest preserves the existing access-description wire.
type AccessSpecDescribeRequest struct {
	Dataset    string          `json:"dataset"`
	Revision   int             `json:"revision,omitempty"`
	Repository string          `json:"repository,omitempty"`
	Pin        json.RawMessage `json:"pin,omitempty"`
}
