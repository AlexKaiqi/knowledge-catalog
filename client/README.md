# KC Client identity and authentication

`client` is the caller-side boundary for KC Server and other remote systems.
It owns three separate concepts:

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

`Client.Do` can authenticate a request to KC or another system. Catalog,
Knowledge, Workspace Files, Writer, Governance, Identity, Admin and Operations
are separate typed clients; there is no arbitrary verb invocation. Every call
re-reads the current
client session and propagates W3C trace context without adding
identity or secrets to baggage.

`MemorySessionStore` intentionally forgets credentials when the process exits.
Applications that need durable login must provide a `SessionStore` backed by an
OS keychain or Agent credential store; credentials must never enter Catalog,
Repository, Workspace pins, logs, or trace baggage.

`CatalogService.CreateRepository` accepts an explicit Catalog ID and
`RepositoryCreateRequest{Repository, CommandID}`. It calls the managed creation
resource without discovering Catalogs. Storage allocation and the authenticated
creator's initial repository-scoped grants follow Server deployment policy;
the request cannot choose an endpoint, credential, creator, or grant list.
The result preserves the durable `APPLIED` / `REPLAYED` status. Retry with the
same command ID and coordinates. `AttachRepository` keeps its separate contract
of validating an existing configured authority before Catalog registration.

Knowledge requests accept either `repository` with `ref`/`commit`, a named
`workspace`, or an unpublished `definition`. `pin` carries the fixed resolved
coordinates; `definition` supplies the membership and layout needed to validate
a temporary pin. Replay does not resolve selectors again or publish a Workspace.
`ResolveDefinition` preserves an optional caller-provided Workspace label in
the definition and pin. That label never borrows a published Workspace's grants;
temporary resolve/consume requires Catalog-scoped or every selected member's
current grants. Replay supplies the labeled `definition` and `pin`, without a
separate named `workspace` selector.
Every composed request still evaluates current `workspace.consume` and member read grants;
a pin is not an authorization token.

The CLI exports temporary task pins with the ordinary `ResolvedWorkspace` fields
plus `definition` and the logical `catalog` ID. `knowledge ... --pin task.json`
restores this input and sends separate typed `definition` and `pin` fields. A
named pin retains its existing JSON shape. Credentials and source locations do
not enter either pin format.

`CatalogService.ConnectRepository` is the typed, explicit connection operation
for an existing Gitea authority at a deployment-approved provider. The request
contains logical repository identity, provider URL and a private credential;
it cannot choose a creator, grant list or server-local path. The response has
management URL and fixed initial head. `RepositoryConnection`,
`CheckRepositoryConnection` and `RotateRepositoryConnection` provide owner
management. Rotation changes only the credential after validating the same
provider repository identity. Request bodies containing credentials must never
be journaled or reflected. The CLI reads these secrets from `--credential-file`.

Catalog discovery uses the same algebra: read `discoveryWorkspaceId` from
`CatalogService.Show`, call `ResolveWorkspace` with `CatalogDiscovery: true`,
then call `KnowledgeService.Search` with that workspace, its fixed `Pin`, and
`CatalogDiscovery: true`. Server validates the exact deployment-selected
workspace before using current `catalog.read` as admission. Arbitrary
workspaces, temporary definitions, repository bases and unpinned searches
cannot use this context. Result delivery still checks current repository
`knowledge.read`; discovery does not authorize exact reads or file bytes.
