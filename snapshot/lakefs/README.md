# snapshot/lakefs

LakeFS layer ⓪ adapter. It implements the same `snapshot.Store` capability set
as the file-backed Gitea adapter while keeping lakeFS and object-store details
behind the Server composition root.

## Contract

- DSN: `http(s)://host[/prefix]/repository`.
- Managed business Graveler ids are the `--name` slug (`table-meta`).
  Owner is not part of the id. Storage namespace is `{root}/{name}`.
  `kc-` is reserved for platform repositories (`kc-catalog`, `kc-system`).
  Allocation is the ledger token only.
- Credential: `KC_LAKEFS_CREDENTIAL=access-key:secret-key`, or a Server-private
  per-repository credential supplied while assembling a connection.
- lakeFS API baseline: v1.86 REST shapes.
- Branch ids: KC `refs/heads/<name>` maps to lakeFS branch `<name>` with `/`
  encoded as `--` (lakeFS branch ids cannot contain slashes). `refs/heads/main`
  stays `main`.
- Expected-old publication uses stock lakeFS operations: atomic creation of a
  hidden per-target lock branch serializes KC publishers; the holder rechecks
  `expected`, hard-resets the clean target to the reviewed candidate, verifies,
  then releases the lock. A crash before publication leaves the lock in place
  and fails later writers closed; replay after a completed reset cleans the
  stale lock.

Deployment policy must make KC's service identity the only writer of published
and `kc-*` branches. An out-of-band hard reset bypasses Writer and invalidates
the lock proof. Vanilla merge is not used because it creates a new commit while
the KC contract requires the published ref to equal the reviewed candidate.

## Data plane

Writes use lakeFS `getPhysicalAddress(presign=true)`, upload bytes directly to
the backing S3-compatible store, then call `linkPhysicalAddress`. Reads request
a presigned object response and follow it from inside KC Server. Presigned URLs
and physical addresses never appear in exported APIs and are never returned to
Clients, Collectors or CI.

Tencent COS, Ceph, MinIO and S3 are backing data planes, not independent
`snapshot.Store` implementations: lakeFS/Graveler supplies the immutable
commit graph, refs and metadata CAS that a plain object bucket does not.

## Write concurrency

`ApplyTreeCommit` stages object bytes onto the private wip branch with a
bounded worker pool (default 32, `KC_LAKEFS_STAGE_CONCURRENCY`). Staging is
independent per path; the first failure stops new staging and the apply
returns it after in-flight writes finish. Commit and publication remain
single-threaded, so expected-old CAS and the hidden lock branch protocol are
unchanged. Object staging keeps the same three-call presigned dance per
object; the pool amortizes round-trip latency, not bytes.

Commit existence checks keep a bounded positive-only cache: lakeFS commits
are immutable, so a known commit never needs re-probing per read. Negative
results are never cached, because a commit may be created concurrently. The
cache holds coordinate identity only — a transport cache per
[`STORE_ADAPTERS.md`](../../docs/STORE_ADAPTERS.md) `CA-01` — never knowledge
semantics.

## Incremental and recovery behavior

- `ChangedPaths` uses paginated `two_dot` diff between immutable commit IDs.
- `CommitHistory` follows first-parent history.
- lakeFS Actions/webhooks are optional latency hints only. The projection
  controller recovers from published HEAD reconciliation.
- Commit metadata records `kc.command_id` when the caller supplies a request
  identity.
- Authority branches must not use retention that deletes replaced committed
  objects. Old `{repository, commit}` coordinates must remain readable.

The package runs the shared Repository contract and cross-provider parity
contract against a protocol-faithful fake lakeFS service. Production capacity,
COS compatibility and multi-instance behavior still require deployment tests.

## Local deployment

`./scripts/kc-snapshot-plane.sh local|online` selects the data plane for adapter
experiments: local (`./scripts/system-lakefs.sh up`) is lakeFS with an embedded
KV plus MinIO, and `kc serve` on the host. Online points lakeFS at COS using
`~/.config/kc/cos-online.env` (keys stay out of git). Homes and ports are
isolated; switching does not rewrite Catalog semantics. COS is the same
blockstore with a different endpoint: do not add a COS SDK to this package.
Pin `treeverse/lakefs:1.86.0` to match the REST baseline above.

The first-stage deploy substitute — lakeFS on PostgreSQL, MinIO standing in for
COS, OpenSearch, containerized `kc-server`, and one observability image — is
`./scripts/deploy/deploy.sh up`. That topology is the local stand-in for the
production companion list in [`docs/SERVICE_ARCHITECTURE.md`](../../docs/SERVICE_ARCHITECTURE.md)
§11.1; it is not a capacity qualification.
