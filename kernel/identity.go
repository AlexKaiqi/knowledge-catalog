package kernel

// RepositoryID is kr://<org>/<scope>/<name>. It names a Snapshot (layer ⓪).
// Catalog repositories and workspace pins use this coordinate.
type RepositoryID string

// CommitID is an immutable authority snapshot version.
type CommitID string

// Digest is a sha256 of a canonical value.
type Digest string
