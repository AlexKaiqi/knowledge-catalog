package cli

import (
	"fmt"
	"strings"
)

// Help is the operator-facing verb list. It changes when the CLI surface
// changes, which is a different reason than any handler changes, so it lives
// in its own file.
const Help = `kc — Knowledge Catalog CLI (protocol verbs)

Role guides: kc help consumer | kc help provider | kc help governor

Workspace
  kc help [--topic consumer|provider|governor] role guide or full protocol surface
  kc init --home <dir> [--catalog <id>]       first Catalog. id is kr://<org>/<name> or <org>/<name>
                                              (omit → kr://local/catalog). Output: {catalog}.
                                              看它：kc read --catalog / kc audit
  kc catalog-add --home <dir> --catalog <id>  another Catalog (own registry git under catalogs/)
  kc register --home <dir> [--catalog <id>] --repo <id>
                                              register an attached Repository in another Catalog
  kc status --home <dir>                      catalogs + attached repositories + workspaces + local stores
  kc status --home <dir> --workspace <id> [--to <dir>]
                                              per-mount git status of a Loom checkout
  kc read --catalog [<id>]                    this Catalog's current combination space
                                              Output: CatalogState {catalogId, repositories, workspaces}
                                              Not knowledge. History is kc audit.
  kc serve --home <dir> [--listen 127.0.0.1:7380]
           [--auth gitea --auth-url <origin> [--auth-admin <principal>]...]
           [--resource-access-url <origin>]
                                              API-only HTTP facade for dsh-plugin and service clients.
                                              There is no KC browser UI; GET / returns 404.
                                              Local mode: X-Kc-As → --as.
                                              Gitea mode: Authorization is verified at /api/v1/user;
                                              principal is gitea:<numeric-id>; X-Kc-As is rejected.
                                              X-Kc-Request-Id → --request-id in either mode.
                                              traceparent/tracestate carry standard trace context;
                                              X-Kc-Trace/Span-Id are legacy fallback.
                                              resource-access-url configures an independent
                                              resource-access/v1 runtime for State Binding READ;
                                              KC_RESOURCE_ACCESS_URL is the environment equivalent.
                                              GET /livez, /readyz[/<surface>], /metrics are management endpoints.
  kc audit --home <dir> [--catalog <id>] [--cmd <verb>] [--limit N]
                                              Catalog 登记表 git 历史（define-workspace / register / retire-workspace）
  kc audit --layer kc|system                  本机过程账：audit.jsonl / system.jsonl
  kc access-log    --home <dir> [--filter-principal <id>] [--filter-on-behalf-of <id>]
                    [--action <verb>] [--trace-id <id>] [--repo <id>] [--object <id>] [--limit N]
                    Durable access evidence: who accessed which pinned knowledge and when.
  kc trace         --home <dir> --trace-id <id>
                    Knowledge-system trace: correlated access spans plus recorded feedback.
  kc hitmap        --home <dir> [--filter-principal <id>] [--filter-on-behalf-of <id>]
                    [--action <verb>] [--repo <id>] [--object <id>] [--limit N]
                    Derived access counts by repository + commit + object/address; never Canonical.
  kc record-feedback --home <dir> --workspace <id> --trace-id <id>
                    --outcome accepted|rejected|corrected|helpful|unhelpful [--message <text>]
                    Append Agent/user feedback to the same trace without writing a Knowledge Repository.

Repository (authority store; Catalogs combine these, do not own them)
  kc repo-add --home <dir> --repo <kr://...> [--driver filegit|dolt|gitea]
                                              [--dsn <url>] [--dir <git>] [--link <git-url>]
                                              default --driver is stores.yaml repository (filegit on local profile).
                                              gitea --dsn is http(s)://host/owner/name; token is KC_GITEA_TOKEN.
                                              dolt uses KC_DOLT_BIN or a Docker engine; image may be pinned with KC_DOLT_DOCKER_IMAGE.
                                              --dir points at an existing local git (no streams/ created there).
                                              --link clones a git URL into layout.repos (gitea --link is --dsn).
                                              --driver mysql is refused.
                                              --dsn is non-secret. Passwords/tokens are env.
                                              A filesystem --dsn for filegit is --dir.
                    repository id may be positional: kc repo-add kr://acme/personals/alice --link <url>
  kc store-set --home <dir> [--profile local|scale]
                    [--repository filegit|dolt|gitea] [--index none|opensearch]
                    [--repos-dir --catalogs-dir --projections-dir --checkouts-dir]
                    [--driver opensearch|filegit|dolt|gitea] [--host --port --database --user --url --dsn]
                                              engines in stores.yaml; dirs in layout.yaml.
                                              --catalogs-dir is the parent of per-id registry gits.
                                              local: FileGit, no SEARCH projection (READ/VFS only).
                                              scale: Dolt + OpenSearch.
  kc store-ls       --home <dir>              layout.yaml + stores.yaml (never prints secrets)
  content write: put / commit --changeset take --repo
  content consume: read / list / search / log / checkout take --workspace (no --repo/--commit/--ref)

Writer (mutates one Repository; Catalog does not store knowledge)
  kc put            --home <dir> --command-id <id> --repo <id> --object <id>
                    [--aspect <name>] [--member <key>] [--file <json>|--value <json>]
                    [--ref refs/heads/main] [--expected <commit>] [--schema-ref <ref>]
					[--value-source <json>]  snapshot(default) or Aspect Binding declaration
                    [--if-absent | --if-digest <digest>]
                    [--origin-kind SOURCE] [--source-ref <s>] [--actor-ref <s>]
                    [--input-workspace-version <ref> --algorithm-spec|--algorithm-model|--algorithm-hash]
                    PUT one Address, then COMMIT. Output: CommitReceipt
  kc remove         same targeting flags without value. Output: CommitReceipt
  kc commit         --home <dir> --command-id <id> --changeset <file.json>
                    ChangeSet or ingest preview. Omits base/expected → current Ref. Output: CommitReceipt
  kc commit         --home <dir> --command-id <id> --workspace <id> [--to <dir>] [--message <text>]
                    Workspace write-back: dirty files in a CheckoutMounts tree, routed by
                    mount, one RawWrite per repository (command-id is suffixed :repo).
                    Output: {workspaceId, commits}. No write grant → FORBIDDEN listing those files.
  kc ingest         --home <dir> --repo <id> --dir <path> [--out <changeset.json>]
                    Preview only; reports identity/schema/search diagnostics.
                    frontmatter object_id wins. Then kc commit --changeset
                    There is deliberately no kc reconcile/connector-run: an external
                    connector uses connector.Preview, then submits the ChangeSet here.
  kc receipt        --home <dir> --command-id <id>
                    Lookup Writer idempotency log. Output: IdempotencyEntry
Consumer (Workspace serving: --workspace; do not pass --repo / --commit / --ref)
	Each command resolves Repository selectors to commits once at its start. Use
  resolve --workspace > pin.json and --pin pin.json to preserve the same
  coordinates across commands; a fresh command intentionally sees new heads.
  Every Workspace consumer verb accepts --pin <ResolvedWorkspace.json|inline-json>.
  kc read           --home <dir> [--catalog <id>] --workspace <id> --object <id>
                    optional --aspect / --include / --exclude
                    Output: ReadResult[] (FederatedValue + observations; State Binding hydrated, union/no override)
                    Bound State requires --resource-access-url / KC_RESOURCE_ACCESS_URL; otherwise CAPABILITY_UNSATISFIED.
                    Stream is not an ordinary READ value. VFS/checkout remain raw Snapshot declarations.
  kc list           --home <dir> [--catalog <id>] --workspace <id>
                    [--limit N] [--continuation <opaque>]
                    Output: {values: ReadResult[], continuation?, exhausted}; State Bindings use logical hydrate.
  kc relations      --home <dir> [--catalog <id>] --workspace <id> --object <id>
                    [--relation-type <type>] [--role <endpoint-role>]
                    One-hop Canonical Relations at this Workspace pin; both endpoints read the same relation object.
  kc search         --home <dir> [--catalog <id>] --workspace <id>
                    [--query <text>] [--match path=text] [--match-mode AllTerms|AnyTerms|Phrase]
                    [--eq|--neq|--gt|--gte|--lt|--lte path=value]
                    [--in path=v1,v2] [--exists|--missing path] [--prefix path=value]
					[--sort path[:asc|:desc]] [--limit N] [--continuation <opaque>]
					Output: SearchResult {searchView, completeness, claims, hits}; every hit rereads pinned
					Canonical and hydrates State Bindings with per-hit observation basis.
					CAPABILITY_UNSATISFIED: run describe-access; schema/* must declare the required access.
  kc provenance     --home <dir> [--catalog <id>] --workspace <id> --object <id>
                    Output: ProvenanceTrace[]
  kc log            --home <dir> [--catalog <id>] --workspace <id> --object <id>
                    Output: ObjectLog[]  (object history at the resolved commits; not Catalog git)
  kc describe-schema --home <dir> [--catalog <id>] --workspace <id> [--object]
                    Output: SchemaReport[]
  kc resolve        --home <dir> [--catalog <id>] --workspace <id> [--object <id>] [--pin <file>]
					no --object: ResolvedWorkspace pin {仓→commit, pinId}
                    with --object: Resolution[]
                    --pin <ResolvedWorkspace.json> replays that pin instead of resolving selectors.
	  kc resolve-binding --home <dir> [--catalog <id>] --workspace <id> --object <id> --aspect <name>
					Output: ResolvedBinding[] at the Workspace pin; declares state/stream access, never invokes it
  kc checkout       --home <dir> [--catalog <id>] --workspace <id> [--to <dir>] [--as <who>]
                    Mount recipe: writable git worktrees at --to (default layout.checkouts/<workspace>).
                    Federated-read recipe (no Path): read-only grep tree (仓/object_id).
                    --as: mounts without a read grant are skipped, not written to disk.
                    Output (mounts): {workspaceId}, dir, mounts}. The root member's
                    .kc-workspace.yaml is ordinary git content (it hitchhikes);
                    .kc-pin.json is checkout-local and excluded from git.
                    Re-run is Sync, not checkout again.
  kc sync           --home <dir> [--catalog <id>] --workspace <id> [--to <dir>]
                    Advance an existing mount checkout per mount. Output: {workspaceId}, dir, mounts}
  kc inspect        --home <dir> [--catalog <id>] --workspace <id>
					Output: CatalogState + pin + AccessPlan + indexes at this pin (not live describe-index)

Virtual filesystem (raw path read/write over a Workspace's composed tree; no
checkout on disk — TreeStore lifted to path routing, not object_id reads)
  kc vfs-read       --home <dir> [--catalog <id>] --workspace <id> --path <virtual/path>
                    Routes path to its owning mount (RouteMount), reads raw bytes there
                    at this ResolveWorkspace's pin. Output: {path, repository, commit, encoding, content}
                    content is base64; encoding names it so a caller never guesses.
  kc vfs-list       --home <dir> [--catalog <id>] --workspace <id> [--prefix <p>]
                    Every path across every mount, translated back to its virtual path.
                    A member with no TreeStore is left out, not an error. Output:
                    {entries, mounts}; mounts explains path -> repository/selector/subPath/commit.
  kc vfs-write      --home <dir> [--catalog <id>] --workspace <id> --command-id <id> --path <virtual/path>
                    [--content <base64> | --remove] [--base <commit>] [--expected <commit>]
                    [--ref refs/heads/main] [--message <text>]
                    Routes path, then RawWrite against the routed repository (not --repo:
                    the target is a fact of the recipe, permission-checked once routed).
                    --base pins the precondition to what vfs-read/vfs-list already returned;
                    omit only for a first write. Without it, retrying --command-id after the
                    ref moved is IDEMPOTENCY_CONFLICT, not a replay — pin --base to get NON_FAST_FORWARD.
                    Output: CommitReceipt

Reader (maintainer: must name --repo and --commit or --ref)
  kc resolve        --home <dir> --repo <id> --object <id> (--commit <id>|--ref <ref>)
                    Output: Resolution (no body)
  kc read           same + optional --aspect / --include / --exclude
                    Output: KnowledgeValue
  kc provenance     same as resolve
                    Output: ProvenanceTrace { repository, commit, objectId, chain }
	  kc resolve-binding --home <dir> --repo <id> --object <id> --aspect <name> (--commit <id>|--ref <ref>)
					Output: ResolvedBinding at the named Snapshot declaration commit
  kc list           --home <dir> --repo <id> (--commit <id>|--ref <ref>)
                    [--limit N] [--continuation <opaque>]
                    Output: {values: KnowledgeValue[], continuation?, exhausted}
  kc relations      --home <dir> --repo <id> --object <id> (--commit <id>|--ref <ref>)
                    [--relation-type <type>] [--role <endpoint-role>]
                    Reference endpoint scan; returned Relation bodies are Canonical at the named commit.
  kc describe-schema --home <dir> --repo <id> (--commit <id>|--ref <ref>) [--object]
                    Output: SchemaReport (schema/* AccessHints; --object follows schema_ref)
  kc search         --home <dir> --repo <id>
                    [--query <text>] [--match path=text] [--match-mode AllTerms|AnyTerms|Phrase]
                    [--eq|--neq|--gt|--gte|--lt|--lte path=value]
                    [--in path=v1,v2] [--exists|--missing path] [--prefix path=value]
                    [--commit <id>|--ref <ref>]  (defaults to index basis; Ensure if named)
					[--sort path[:asc|:desc]] [--limit N] [--continuation <opaque>]
					Output: SearchResult (atomic clauses, implicit AND; hydrate Canonical)
  kc describe-index --home <dir> --repo <id>
                    Output: IndexDescriptor (basis / lag / compiled AccessHints)
  kc index-sync     --home <dir> --repo <id> (--commit <id>|--ref <ref>)
                    Output: IndexSync (incremental if possible, else rebuild).
                    Under kc serve with resource-access configured, also performs the
                    State notify-and-pull refresh and returns {snapshot,state}.
  kc log            --home <dir> --repo <id> --object <id> (--commit <id>|--ref <ref>)
                    Output: ObjectRevision[]  (collapsed history)
  kc diff           --home <dir> --repo <id> --object <id> --from <commit> --to <commit>
                    Output: ObjectDiff

Catalog (combination space; --catalog selects which; omit when only one)
  kc define-workspace --home <dir> [--catalog <id>] --workspace <id> --revision <n>
                    --source <repo>=<selector>[@<path>[@<subPath>]]   (repeatable; Repository must be attached)
                    no @: federated read only. @ alone: mount at root (Path ""). @refs/x: nested mount.
                    Mount recipes also write .kc-workspace.yaml into the root
                    member (or <home>/workspaces/<id>.yaml when there is no root).
                    --file <yaml> / --from-repo <id> import that hitchhiking recipe.
                    --base-rev <repo>=<commit> (repeatable) is recipe-layer CAS:
                    resolve fails NON_FAST_FORWARD if that selector has moved.
                    Output: WorkspaceDefinition + recipeFile
  kc overlay        --home <dir> --workspace <id> [--file <yaml>|--clear]
                    Local overlay (Android repo local_manifests). Only this --home
                    and this --as. Adds, replaces, or removes mounts without
                    rewriting the shared recipe. No --file prints the effective sources.
	  kc describe-access --home <dir> [--catalog <id>] --workspace <id>
					Output: AccessPlan with one logical AccessSpec per pinned Repository.
  kc retire-workspace --home <dir> [--catalog <id>] --workspace <id>
  kc archive-catalog --home <dir> [--catalog <id>]
  kc archive-repo   --home <dir> --repo <id>

Access (empty allow.json = workspace owner; --as must match a rule)
  kc allow          --home <dir> --principal <who> --cmd <verbs>
					(--repo <id>|--catalog <id>) [optional --ref --object --aspect --workspace]
  kc revoke         --home <dir> --id <rule-id>
  kc allowed        --home <dir> [--principal|--as] [--cmd ...]
  kc whoami         --home <dir> [--as]
  verbs also take     --as <principal> [--on-behalf-of <user>] [--request-id <token>]
                    [--trace-id <id> --span-id <id> --parent-span-id <id>]
                    Authentication stays outside kc; these are trusted identity/trace assertions.
                    Catalog / 仓库的 git commit 记下 principal / request-id / 命中的 ruleId。

Hook (outbound; .kc/hooks.json. pre = --run fail closed; post = pointers, must not roll back)
  kc hook-add       --home <dir> --on <cmd> --phase pre|post
                    [--repo <id>|--catalog <id>] (--run <path>|--url <url>)
  kc hook-ls        --home <dir> [--on] [--repo|--catalog]
  kc hook-rm        --home <dir> --id <hook-id>

Gate (inbound checklist on merge; .kc/gates.json. not a hook)
  kc gate-add       --home <dir> --on merge --require validate,suite:<name>
                    --repo <id>
  kc gate-ls        --home <dir> [--on] [--repo|--catalog]
  kc gate-rm        --home <dir> --id <gate-id>

Control Plane (content still goes through Writer)
  kc propose        --home <dir> --proposal-id <id> --repo <id>
                    --target <ref> --candidate <ref>
                    PUT flags (--object --value/--file [--aspect]) or --changeset
                    Output: Proposal  (writes candidate Ref only)
  kc preview        --home <dir> [--catalog <id>] --proposal <id> --workspace <id>
                    Output: Preview (Workspace + overlay; stored in ControlState)
  kc validate       --home <dir> [--catalog <id>] --preview <id>
                    Structural check (attached repositories + commits exist), then records outcome
  kc record-validation --home <dir> [--catalog <id>] --preview <id>
                    --suite <rev> --outcome PASSED|FAILED
                    Records an external suite outcome; does not run tests
  kc merge          --home <dir> --proposal <id> --preview <id> [--validation <id>]
                    Fast-forwards target Ref. Next read --workspace follows the published branch.
                    Authorization derives Repository/target Ref from the stored Proposal.
                    With a matching gate, all stored evidence on this Preview is checked;
                    without a gate, one PASSED --validation is required.
                    Output includes repository, targetRef and gate {status,basis,required}.

Default --home is ./.kc
Connection: .kc/layout.yaml (this machine's dirs) + .kc/stores.yaml (engines + hosts).
Two store stacks, separate public interfaces (snapshot.Store/TreeStore authority + knowledge Reader/Writer + index.Engine):
	local  — FileGit Snapshot authority; no SEARCH projection (READ/VFS only).
	scale  — Dolt Snapshot + OpenSearch projection.
Catalog registry is always FileGit under layout.catalogs/<encoded-id>.
OpenSearch is an optional stores.yaml section; secrets are
KC_ELASTICSEARCH_PASSWORD or KC_ELASTICSEARCH_API_KEY. --dsn / stores.yaml must not contain passwords.
kc store-set writes both files; repo-add --dsn merges non-secret URL fields into stores.yaml.
Index: "index: none" (SEARCH returns CAPABILITY_UNSATISFIED) | "index: opensearch" (service projection). Snapshot candidates are reread from Canonical. Queries touching State Binding fields refresh the separate dynamic projection and hydrate hits from the same published observation basis.
kc serve pins that home; POST /v1/<verb> JSON uses CLI flag names without the leading --.
Without --auth it is a local owner facade: X-Kc-As is --as. With --auth gitea,
send Authorization: Bearer|token|Basic; credentials are verified by Gitea and X-Kc-As is disabled.
Gitea site admins and repeated --auth-admin principals may run local administration verbs.
Use TLS at the reverse proxy; incoming user credentials are distinct from the repository adapter's KC_GITEA_TOKEN.
X-Kc-Request-Id is --request-id.
Writes require --command-id (retry = same id + same body; content change = new id).
First Catalog is layout.catalogs/<encoded-id> (default <home>/catalogs/…);
more catalogs are siblings under the same parent. --catalog selects which; omit it when there is only one.
FileGit repositories are layout.repos/<encoded-id>. Durable service projections live outside the Repository authority.
Workspace checkout trees are layout.checkouts/<workspace> (discardable grep Provider; not authority).
Writer command ledger: <home>/writer.db (legacy writer.json is migrated on first open)
kc log: <home>/audit.jsonl          本机 facade（kc audit --layer kc）
system log: <home>/system.jsonl     本机协议过程账（kc audit --layer system）
Catalog 当前态: kc read --catalog. 历史: 登记表 git（kc audit）
`

const ConsumerHelp = `kc help consumer — consume a frozen Workspace pin

Mental model
  Repository       versioned authority containing knowledge objects
  Catalog          registers Repositories and Workspace recipes; stores no content
  Workspace        consumer composition recipe; it is not another Repository
  ResolvedWorkspace/pin
                   one immutable {repository -> commit} view for this task

Choose an entry
  known object     knowledge_read / kc read: exact Canonical content at the pin
  unknown object   knowledge_search, then Canonical read; or bounded knowledge_list
  mounted files    kcfs + ordinary rg for read-only browsing, not another authority

Common questions
  Upstream changed?  The current task stays stable; a new resolution sees new commits.
  SEARCH unavailable? index:none is intentional locally. Use list/rg, or configure
                      the service OpenSearch projection; do not add SQLite/memory.
  Which history?      audit = Catalog, log = object revisions, provenance = origins.

Discover
  kc read --catalog
  # choose a workspaceId from CatalogState.workspaces; do not guess it

Shortest reliable flow
  kc resolve --workspace <id> > pin.json
  kc search --workspace <id> --pin pin.json --query <text>
  kc read --workspace <id> --pin pin.json --object <object_id>
  kc provenance --workspace <id> --pin pin.json --object <object_id>

Interpretation (SearchResult.completeness)
  complete       every authorized member satisfied the SEARCH
  partial        inspect claims; a member was unsupported or authorization-clipped
  empty hits     a valid zero result, unlike CAPABILITY_UNSATISFIED
  FORBIDDEN      a bare READ cannot honestly represent an incomplete Workspace

Diagnosis
  kc describe-access --workspace <id>
  kc inspect --workspace <id>
  kc whoami --as <principal>

Authenticated HTTP
  Authorization determines principal; do not send --as / "as" yourself.
  POST /v1/read with {"catalog":true} discovers the current Catalog.
  CLI flag names become JSON keys; "pin" is ResolvedWorkspace JSON encoded as a string.
  X-Kc-Request-Id supplies the request-id in authenticated service mode.

All Workspace consumer verbs accept --pin <file|inline-json>.
Use kc help for the full protocol surface.
`

const ProviderHelp = `kc help provider — admit and publish knowledge

Mental model
  Repository is the write and governance boundary. object_id is stable knowledge
  identity, not a path/URN/source key. Keep source-key mapping in the provider.
  source-ref records origin but does not replace that mapping. Publishing needs
  a target Repository, not a Workspace; Workspace is a later consumer recipe.
  Do not require the mapping itself to be a Catalog object unless deliberately modeled.
  Schema is versioned knowledge in schema/*, not source code or local config.

Provider contract
  collect current source state outside kc → map to Addresses/ChangeSet → review →
  commit or propose through Writer. Attach stable source-ref provenance. Never edit
  Repository files or git directly, and do not put a source client inside Writer.

Smallest readable publish
  kc repo-add --repo <kr://...>
  kc put --command-id <stable-id> --repo <id> --object <object_id> \
    --value <json> --origin-kind SOURCE --source-ref <stable-source-ref>
  kc read --repo <id> --object <object_id> --ref refs/heads/main
  kc provenance --repo <id> --object <object_id> --ref refs/heads/main

Searchable publish
  kc put --command-id <schema-id> --repo <id> --object schema/<name> --value <schema-json>
  kc put --command-id <stable-id> --repo <id> --object <object_id> \
    --schema-ref schema/<name> --value <json> \
    --origin-kind SOURCE --source-ref <stable-source-ref>
  kc describe-schema --repo <id> --ref refs/heads/main --object <object_id>

Files or prepared knowledge
  kc ingest --repo <id> --dir <drafts> --out changeset.json
  # ingest never writes; review stdout diagnostics; --out is only the reusable ChangeSet
  kc commit --command-id <stable-id> --changeset changeset.json

Workspace is a consumer composition, not a prerequisite for publishing.

Authenticated service onboarding
  repo-add attaches/registers a Repository; it deliberately grants no knowledge access.
  Before PUT, an operator must allow put/remove/commit and the required read verbs
  for the authenticated principal. --auth-admin only bypasses local administration verbs.
  Authorization determines principal; X-Kc-Request-Id supplies request-id.

External systems stay outside kc:
  collect → connector.Preview → ChangeSet → commit/propose.
There is deliberately no connector-run or second Write Surface.
Use kc help for the full protocol surface.
`

const GovernorHelp = `kc help governor — compose, authorize, and publish governed changes

Mental model
  Catalog is the composition/control plane, not a knowledge Repository.
  Workspace composes member Repository selectors; resolving it freezes one pin.
  Repository remains the authorization, write, and governance boundary.

Boundaries
  One Workspace permission never implies member Repository permission.
  One task never follows latest after resolution. There is no cross-Repository
  atomic write, and a permissions Aspect does not enforce source-system access.

Compose
  kc define-workspace --workspace <id> --revision <n> --source <repo>=<selector>
  kc inspect --workspace <id>

Authorize
  kc allow --principal <who> --cmd read-workspace --catalog <id> --workspace <id>
  kc allow --principal <who> --cmd read --repo <member-repo>
  kc allowed --principal <who>

Governed publish
  kc propose → kc preview → kc validate / record-validation → kc merge

Audit
  kc read --catalog
  kc audit
  kc access-log / kc trace / kc hitmap

Workspace permission never grants member Repository permission implicitly.
Use kc help for the full protocol surface.
`

func helpFor(topic string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(topic)) {
	case "":
		return Help, nil
	case "consumer":
		return ConsumerHelp, nil
	case "provider":
		return ProviderHelp, nil
	case "governor":
		return GovernorHelp, nil
	default:
		return "", fmt.Errorf("unknown help topic %s; want consumer, provider, or governor", topic)
	}
}
