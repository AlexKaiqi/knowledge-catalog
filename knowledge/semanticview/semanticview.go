// Package semanticview is the public shim for the semantic rendering view
// (CAP-ORG-01 B11, ruling C): the implementation lives in
// knowledge/internal/semanticviewimpl and only this shim plus the knowledge
// tree may import it, keeping the app-layer import path stable.
package semanticview

import "kc/knowledge/internal/semanticviewimpl"

type (
	// Metadata describes one rendered semantic view file.
	Metadata = semanticviewimpl.Metadata
)

var (
	Path   = semanticviewimpl.Path
	Render = semanticviewimpl.Render
	Slug   = semanticviewimpl.Slug
)
