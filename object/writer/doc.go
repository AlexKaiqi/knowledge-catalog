// Package writer applies layer ② Object ChangeSets through COMMIT, opens work
// branches, and fast-forwards a reviewed branch onto its target through MERGE.
// Literal path-byte commits belong to snapshot/treewriter.
package writer
