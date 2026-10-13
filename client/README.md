# KC Client identity and authentication

`client` is the caller-side boundary for KC Server and other remote systems.
One service owns one registration space; callers never create, select or switch a Catalog. The Snapshot client addresses inventory at `/snapshot/v1/inventory`, registry audit at `/snapshot/v1/audit`, Datasets at `/snapshot/v1/datasets`, registered members at `/snapshot/v1/repositories`, and connections at `/snapshot/v1/connections`. It owns three separate concepts:

- `Identity`: `principal` plus optional `onBehalfOf`, reused by authorization
  and access evidence;
- `Authentication`: an opaque credential value that protocol packages never
  inspect;
- `Session`: client-local login state, not a server resource and not a
  `WorkspaceSession`.

The default `PassThroughAuthenticator` only validates field shape and may
forward both `Authorization` and `X-Kc-As`. That is a test seam, not a pairing.
CLI local pairing (`--auth local`) sends only `X-Kc-As`. Token pairing
(`--auth taihu|gitea`) sends only `Authorization`. Mixing both on one request
is fail-closed. A production authenticator verifies login, refresh or exchange
in `AuthenticateRequest`, and never client-asserts identity headers when the
server derives identity from the verified credential.

`Client.Do` can authenticate a request to KC or another system. Snapshot,
Knowledge, Workspace Files, Writer, Governance, Identity, Admin and Operations
are separate typed clients; there is no arbitrary verb invocation. Every call
re-reads the current
client session and propagates W3C trace context without adding
identity or secrets to baggage.

Typed request bodies larger than 512 KiB travel as `Content-Encoding: gzip`
(stdlib `compress/gzip`); smaller bodies stay plain. The service facade
decompresses transparently and enforces its size caps on the decompressed
bytes, so compression never weakens the body limit. Scale ChangeSets stop
paying the raw-JSON wire cost without changing the ChangeSet contract or any
endpoint shape. The writer commit route additionally carries its own larger
body cap (`KC_MAX_COMMIT_REQUEST_BYTES`, default 512 MiB) because legitimate
whole-repository ChangeSets exceed the per-route service default.

Native lakeFS `/api/v1/` requests remain uncompressed, retaining the upstream
wire contract. Native error bodies (`message` / `code`) are decoded without
changing KC error-envelope handling.

`SnapshotService.RepositoryMetadata`, `SetRepositoryMetadata` and
`DeleteRepositoryMetadata` address the actual native lakeFS repository name
at `/api/v1/repositories/{repository}/metadata`; callers never infer that name
from a logical Repository ID. Reads return a string map, sets submit
`{metadata: {...}}` and preserve unspecified keys, and deletes submit
`{keys: [...]}`. Current product authorization and upstream authorization both
apply. The original lakeFS repository remains the only metadata authority.

`KnowledgeSetRequest.Metadata` is an optional `map[string]string`. Dataset
definition publishes the complete object with its revision; omission or `{}`
publishes no metadata. Dataset inventory and selected historical detail carry
the accepted metadata. This capability does not read or interpret files.

`MemorySessionStore` intentionally forgets credentials when the process exits.
Applications that need durable login must provide a `SessionStore` backed by an
OS keychain or Agent credential store; credentials must never enter registration state,
Repository, Workspace pins, logs, or trace baggage.

`SnapshotService.CreateRepository` accepts
`RepositoryCreateRequest{Repository, CommandID}`. It calls the managed creation
resource in the service registration space. Storage allocation and the authenticated
creator's initial repository-scoped grants follow Server deployment policy;
the request cannot choose an endpoint, credential, creator, or grant list.
The result preserves the durable `APPLIED` / `REPLAYED` status. Retry with the
same command ID and coordinates. `AttachRepository` keeps its separate contract
of validating an existing configured authority before member registration.

Knowledge requests accept either `repository` with `ref`/`commit`, a named
`workspace`, or an unpublished `definition`. `pin` carries the fixed resolved
coordinates; `definition` supplies the membership and layout needed to validate
a temporary pin. Replay does not resolve selectors again or publish a Workspace.
`ResolveDefinition` preserves an optional caller-provided Workspace label in
the definition and pin. That label never borrows a published Workspace's grants;
temporary resolve/consume requires service-wide or every selected member's
current grants. Replay supplies the labeled `definition` and `pin`, without a
separate named `workspace` selector.
Dataset knowledge delivery evaluates current `file.read` for its admitted scope;
temporary definitions use the service-wide or every selected member's scope. Direct
repository knowledge requests use their separate repository grants. A pin fixes
coordinates, not authorization, and never freezes grants.

HTTP knowledge requests may still send a protocol `pin` JSON together with an
unpublished `definition`. Product CLI argv rejects `--pin`; consumers pass
`--dataset` or `--repo`. Credentials and source locations do not enter pin
documents.

`WorkspaceFilesService` is the typed Dataset File Gateway: `Mounts`, `Directory`
and `Read` address one published Dataset through `WorkspaceFileCoordinate`
(`dataset`, an optional fixed `pin`, and `repository`/`semantic`
view). Two addressing modes exist and never mix on one request:
mount-relative reads need a non-empty `MountPath` plus a member-relative file
path, while delivered addressing walks the published delivered tree with `Path`
(empty `Path` is the root). The delivered tree is the union of per-file entries
and mount content under their delivered targets; renamed entries keep their
source coordinates invisible except for the delivery target. Every request
re-evaluates current `file.read` for the published Dataset scope — a pin is not an
authorization token — and with a fixed pin the enumeration and bytes stay on
the pin's commits even after a newer version is published.

`SnapshotService.ConnectRepository` is the typed, explicit connection operation
for an existing lakeFS authority at a deployment-approved provider. The request
contains logical repository identity, provider URL and a private credential;
it cannot choose a creator, grant list or server-local path. The response has
management URL and fixed initial head. `RepositoryConnection`,
`CheckRepositoryConnection` and `RotateRepositoryConnection` provide owner
management. Rotation changes only the credential after validating the same
provider repository identity. Request bodies containing credentials must never
be journaled or reflected. The CLI reads these secrets from `--credential-file`.

Inventory discovery uses the same algebra: read `discoveryWorkspaceId` from
`SnapshotService.Show`, call `ResolveKnowledgeSet` with `InventoryDiscovery: true`,
then call `KnowledgeService.Search` with that workspace, its fixed `Pin`, and
`InventoryDiscovery: true`. Server validates the exact deployment-selected
workspace before using current `inventory.read` as admission. Arbitrary
workspaces, temporary definitions, repository bases and unpinned searches
cannot use this context. Result delivery still checks current repository
`knowledge.read`; discovery does not authorize exact reads or file bytes.
