package kernel

// RepositoryID names a Snapshot (layer ⓪). Catalog members and pins use this
// coordinate. It is either kr://<org>/<name> or a lakeFS Graveler name so a
// managed lakeFS repository is the same id in KC and in lakeFS.
type RepositoryID string

// CommitID is an immutable authority snapshot version.
type CommitID string

// Digest is a sha256 of a canonical value.
type Digest string
