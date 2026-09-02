// Package worktree materializes a resolved Workspace as real git working trees.
//
// Catalog supplies the recipe, the pin, and Snapshot capabilities. This
// package owns host checkout, sync, status, and collecting local writes. It
// is not registry state (docs/COMPOSITION.md §3.3).
package worktree
