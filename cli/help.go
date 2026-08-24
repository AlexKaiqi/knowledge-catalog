package cli

// Help is the operator-facing verb list. It changes when the CLI surface
// changes, which is a different reason than any handler changes, so it lives
// in its own file.
const Help = `kc — Knowledge Catalog CLI (protocol verbs)

Workspace
  kc init --home <dir> [--catalog <id>]       first Catalog. id is kr://<org>/<name> or <org>/<name>
                                              (omit → kr://local/catalog). Output: {catalog}.
                                              看它：kc read --catalog / kc audit
  kc catalog-add --home <dir> --catalog <id>  another Catalog (own registry git under catalogs/)
  kc register --home <dir> [--catalog <id>] --repo <id>
                                              admit a mounted repo into a Catalog
  kc status --home <dir>                      catalogs + mounted repos + workspaces + local stores
  kc status --home <dir> --workspace <id> [--to <dir>]
                                              per-mount git status of a Loom checkout
  kc read --catalog [<id>]                    this Catalog's current combination space
                                              Output: CatalogState {catalogId, repositories, workspaces}
                                              Not knowledge. History is kc audit.
  kc serve --home <dir> [--listen 127.0.0.1:7380]
           [--auth gitea --auth-url <origin> [--auth-admin <principal>]...]
                                              HTTP facade: same verbs as this CLI. UI at GET /
                                              Local mode: X-Kc-As → --as.
                                              Gitea mode: Authorization is verified at /api/v1/user;
                                              principal is gitea:<numeric-id>; X-Kc-As is rejected.
                                              X-Kc-Request-Id → --request-id in either mode.
  kc audit --home <dir> [--catalog <id>] [--cmd <verb>] [--limit N]
                                              Catalog 登记表 git 历史（define-workspace / register / retire-workspace）
  kc audit --layer kc|system                  本机过程账：audit.jsonl / system.jsonl

Repository (authority store; Catalogs combine these, do not own them)
  kc repo-add --home <dir> --repo <kr://...> [--driver filegit|dolt|gitea]
                                              [--dsn <url>] [--dir <git>] [--link <git-url>]
                                              default --driver is stores.yaml repository (filegit on local profile).
                                              gitea --dsn is http(s)://host/owner/name; token is KC_GITEA_TOKEN.
                                              dolt uses KC_DOLT_BIN or a Docker engine; image may be pinned with KC_DOLT_DOCKER_IMAGE.
                                              --dir points at an existing local git (no streams/ created there).
                                              --link clones a git URL into layout.repos (gitea --link is --dsn).
                                              stream is not a snapshot repo.
                                              --driver mysql is refused.
                                              --dsn is non-secret. Passwords/tokens are env.
                                              A filesystem --dsn for filegit is --dir.
  kc mount          same flags as repo-add. Loom-facing name for hanging a git repo.
                    repo id may be positional: kc mount kr://acme/personals/alice --link <url>
  kc store-set --home <dir> [--profile local|scale]
                    [--repository filegit|dolt|gitea] [--index sqlite|elasticsearch] [--cache redis]
                    [--repos-dir --catalogs-dir --projections-dir --checkouts-dir]
                    [--driver redis|elasticsearch|starrocks|filegit|sqlite|dolt|gitea] [--host --port --database --user --url --dsn]
                                              engines in stores.yaml; dirs in layout.yaml.
                                              --catalogs-dir is the parent of per-id registry gits.
                                              local: FileGit + JSONL + SQLite, no Redis.
                                              scale: Dolt + stream + ES + StarRocks + Redis cache (Dolt/stream/SR stubbed).
  kc store-ls       --home <dir>              layout.yaml + stores.yaml (never prints secrets)
  content write: put / commit --changeset / append take --repo
  content consume: read / list / search / log / checkout take --workspace (no --repo/--commit/--ref)

Writer (mutates one Repository; Catalog does not store knowledge)
  kc put            --home <dir> --command-id <id> --repo <id> --object <id>
                    [--aspect <name>] [--member <key>] [--file <json>|--value <json>]
                    [--ref refs/heads/main] [--expected <commit>] [--schema-ref <ref>]
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
                    Preview only (frontmatter object_id wins). Then kc commit --changeset
                    There is deliberately no kc reconcile/connector-run: an external
                    connector uses connector.Preview, then submits the ChangeSet here.
  kc receipt        --home <dir> --command-id <id>
                    Lookup Writer idempotency log. Output: IdempotencyEntry
  kc append         --home <dir> --command-id <id> --repo <id> --stream <name>
                    --event-id <id> [--payload <json>|--file <json>] [--cursor <n>]
                    [--schema-ref <ref>]
                    Empty --cursor is filled from the current stream (retry reuses it)
                    Output: AppendReceipt

Consumer (Workspace serving: --workspace; do not pass --repo / --commit / --ref)
  Each command resolves selectors and AppendCuts once at its start. Use
  resolve --workspace > pin.json and --pin pin.json to preserve the same
  coordinates across commands; a fresh command intentionally sees new heads.
  kc read           --home <dir> [--catalog <id>] --workspace <id> --object <id>
                    optional --aspect / --include / --exclude
                    Output: FederatedValue[]  (KnowledgeValue fields + objectId; union, no override)
  kc list           --home <dir> [--catalog <id>] --workspace <id>
                    Output: FederatedValue[]
  kc search         --home <dir> [--catalog <id>] --workspace <id>
                    [--query <text>] [--match path=text]
                    [--eq|--neq|--gt|--gte|--lt|--lte path=value]
                    [--in path=v1,v2] [--exists path] [--sort path[:asc|:desc]]
                    Output: KnowledgeValue[]  (member indexes at the resolved commits; hydrate Canonical)
  kc provenance     --home <dir> [--catalog <id>] --workspace <id> --object <id>
                    Output: ProvenanceTrace[]
  kc log            --home <dir> [--catalog <id>] --workspace <id> --object <id>
                    Output: ObjectLog[]  (object history at the resolved commits; not Catalog git)
  kc stream         --home <dir> [--catalog <id>] --workspace <id> --stream <name>
                    Output: StreamPage  (cut frozen at ResolveWorkspace; not live head)
  kc describe-schema --home <dir> [--catalog <id>] --workspace <id> [--object]
                    Output: SchemaReport[]
  kc resolve        --home <dir> [--catalog <id>] --workspace <id> [--object <id>] [--pin <file>]
                    no --object: ResolvedWorkspace pin {仓→commit, AppendCuts, pinId}
                    with --object: Resolution[]
                    --pin <ResolvedWorkspace.json> replays that pin instead of resolving selectors.
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
                    Output: CatalogState + pin + IndexPlan + indexes at this pin (not live describe-index)

Virtual filesystem (raw path read/write over a Workspace's composed tree; no
checkout on disk — RawFileStore lifted to path routing, not object_id reads)
  kc vfs-read       --home <dir> [--catalog <id>] --workspace <id> --path <virtual/path>
                    Routes path to its owning mount (RouteMount), reads raw bytes there
                    at this ResolveWorkspace's pin. Output: {path, repository, commit, encoding, content}
                    content is base64; encoding names it so a caller never guesses.
  kc vfs-list       --home <dir> [--catalog <id>] --workspace <id> [--prefix <p>]
                    Every path across every mount, translated back to its virtual path.
                    A member with no RawFileStore is left out, not an error. Output: {entries}
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
  kc stream         --home <dir> --repo <id> --stream <name>
                    [--from-cursor <c>] [--limit N]   continue (resume; cursor is opaque)
                    [--event-id <id>]                 lookup
                    [--since <RFC3339>] [--until <RFC3339>]  durable time window
                    Output: StreamPage
  kc list           --home <dir> --repo <id> (--commit <id>|--ref <ref>)
                    Output: KnowledgeValue[]
  kc describe-schema --home <dir> --repo <id> (--commit <id>|--ref <ref>) [--object]
                    Output: SchemaReport (schema/* AccessHints; --object follows schema_ref)
  kc search         --home <dir> --repo <id>
                    [--query <text>] [--match path=text]
                    [--eq|--neq|--gt|--gte|--lt|--lte path=value]
                    [--in path=v1,v2] [--exists path] [--sort path[:asc|:desc]]
                    [--commit <id>|--ref <ref>]  (defaults to index basis; Ensure if named)
                    Output: KnowledgeValue[]  (atomic clauses, implicit AND; hydrate Canonical)
  kc describe-index --home <dir> --repo <id>
                    Output: IndexDescriptor (basis / lag / compiled AccessHints)
  kc index-sync     --home <dir> --repo <id> (--commit <id>|--ref <ref>)
                    Output: IndexSync (incremental if possible, else rebuild)
  kc log            --home <dir> --repo <id> --object <id> (--commit <id>|--ref <ref>)
                    Output: ObjectRevision[]  (collapsed history)
  kc diff           --home <dir> --repo <id> --object <id> --from <commit> --to <commit>
                    Output: ObjectDiff

Catalog (combination space; --catalog selects which; omit when only one)
  kc define-workspace --home <dir> [--catalog <id>] --workspace <id> --revision <n>
                    --source <repo>=<selector>[@<path>[@<subPath>]]   (repeatable; repo must be mounted)
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
  kc index-plan     --home <dir> [--catalog <id>] --workspace <id>
                    Output: IndexPlan (derived recipe at current selectors). Working index compiles on put/commit/merge.
  kc retire-workspace --home <dir> [--catalog <id>] --workspace <id>
  kc archive-catalog --home <dir> [--catalog <id>]
  kc archive-repo   --home <dir> --repo <id>

Access (empty allow.json = workspace owner; --as must match a rule)
  kc allow          --home <dir> --principal <who> --cmd <verbs>
                    (--repo <id>|--catalog <id>) [optional --ref --object --aspect --stream --workspace]
  kc revoke         --home <dir> --id <rule-id>
  kc allowed        --home <dir> [--principal|--as] [--cmd ...]
  kc whoami         --home <dir> [--as]
  verbs also take     --as <principal> [--request-id <token>]
                    Catalog / 仓库的 git commit 记下 as / request-id / 命中的 ruleId。

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
                    Structural check (mounted repos + commits exist), then records outcome
  kc record-validation --home <dir> [--catalog <id>] --preview <id>
                    --suite <rev> --outcome PASSED|FAILED
                    Records an external suite outcome; does not run tests
  kc merge          --home <dir> --proposal <id> --preview <id> [--validation <id>]
                    Fast-forwards target Ref. Next read --workspace follows the published branch.
                    --validation required unless a matching merge gate is configured.

Default --home is ./.kc
Connection: .kc/layout.yaml (this machine's dirs) + .kc/stores.yaml (engines + hosts).
Two store stacks, same public interfaces (repository.Repository, index.Engine):
  local  — FileGit + JSONL authority, SQLite index. No Redis.
  scale  — Dolt snapshot + ordered-stream APPEND (stubs), ES full-text, StarRocks columns (stub), Redis cache only.
Catalog registry is always FileGit under layout.catalogs/<encoded-id>. Redis is a discardable hot-tail cache, never authority or GT.
profile: local rejects redis as index or cache. scale may set cache: redis.
Managed hosts (redis/elasticsearch/starrocks) are optional stores.yaml sections; secrets are KC_REDIS_PASSWORD,
KC_ELASTICSEARCH_PASSWORD or KC_ELASTICSEARCH_API_KEY, KC_STARROCKS_PASSWORD. --dsn / stores.yaml must not contain passwords.
kc store-set writes both files; repo-add --dsn merges non-secret URL fields into stores.yaml.
Index: "index: sqlite" (local FTS+fields) | "index: elasticsearch" (scale full-text). Redis is not a repository; miss must read the store.
kc serve pins that home; POST /v1/<verb> JSON uses CLI flag names without the leading --.
Without --auth it is a local owner facade: X-Kc-As is --as. With --auth gitea,
send Authorization: Bearer|token|Basic; credentials are verified by Gitea and X-Kc-As is disabled.
Gitea site admins and repeated --auth-admin principals may run local administration verbs.
Use TLS at the reverse proxy; incoming user credentials are distinct from the repository adapter's KC_GITEA_TOKEN.
X-Kc-Request-Id is --request-id.
Writes require --command-id (retry = same id + same body; content change = new id).
First Catalog is layout.catalogs/<encoded-id> (default <home>/catalogs/…);
more catalogs are siblings under the same parent. --catalog selects which; omit it when there is only one.
FileGit repositories are layout.repos/<encoded-id>. SQLite projections are layout.projections.
Workspace checkout trees are layout.checkouts/<workspace> (discardable grep Provider; not authority).
Writer log: <home>/writer.json
kc log: <home>/audit.jsonl          本机 facade（kc audit --layer kc）
system log: <home>/system.jsonl     本机协议过程账（kc audit --layer system）
Catalog 当前态: kc read --catalog. 历史: 登记表 git（kc audit）
`
