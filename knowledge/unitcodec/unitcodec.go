// Package unitcodec is the public shim for the unit codec (CAP-ORG-01 B11,
// ruling C): the implementation lives in knowledge/internal/unitcodecimpl;
// the repofile tree keeps this stable path.
package unitcodec

import "kc/knowledge/internal/unitcodecimpl"

type Unit = unitcodecimpl.Unit

var (
	Assemble               = unitcodecimpl.Assemble
	AssembleKnowledgeValue = unitcodecimpl.AssembleKnowledgeValue
	DeclarationDigest      = unitcodecimpl.DeclarationDigest
	Declarations           = unitcodecimpl.Declarations
)
