---
name: knowledge-catalog
description: Answer Knowledge Catalog usage and concept questions, then operate it from an empty workspace through publishing, consumption, governance, diagnosis, and recovery.
---

# Knowledge Catalog user and operator guide

## Mental model and common questions

Start from the user's job, not from a command name:

- A **Repository** is the versioned authority where knowledge objects live. It
  has commits and refs and is the write/governance boundary.
- A **Catalog** registers Repositories and Workspace definitions. It is the
  composition/control plane, not a content store and not another Repository.
- A **Workspace** is a named recipe that composes one or more Repository
  selectors for consumption. It does not copy or own their knowledge.
- A **ResolvedWorkspace**, also called a pin, resolves that recipe once into
  `{repository -> commit}`. One Agent session reuses one immutable pin, so an
  upstream HEAD change cannot silently change an answer mid-task. A new
  session or explicit re-resolution can observe the new commit.
- An **object_id** is stable knowledge identity. It is not a file path, URN, or
  source-system key. A provider owns the source-key-to-object-ID mapping.
  `source-ref` provenance records where a published value came from; it does
  not replace that mapping table. The mapping stays in the provider/integration
  boundary and is not automatically another Catalog knowledge object; publish
  it only when it is deliberately modeled as knowledge.
- A **Schema** is versioned knowledge stored as a `schema/*` object and
  published through Writer. It is not application source code or local config.

Choose the public surface by intent:

- Exact known object: `knowledge_read`. Natural-language discovery:
  `knowledge_search`; it returns candidates and KC then reads Canonical content
  at the same pin before answering. `knowledge_read` returns Snapshot values at
  that pin and hydrates State Bindings through Knowledge Serving with a separate
  observation basis; missing runtime is an explicit capability failure, not a
  `null` knowledge value. Unknown IDs: bounded `knowledge_list`.
- Mounted `kcfs` files are a read-only human/shell projection for ordinary
  file tools such as `rg`; they are not a second authority or a second Agent
  knowledge API.
- `audit` explains Catalog composition/operation history; `log` explains an
  object's digest revisions; `provenance` explains the object's recorded
  origin envelopes. None substitutes for the others. `knowledge_provenance` is
  the typed Workspace provenance tool; `audit` and `log` use the generic `kc`
  tool. There is no `knowledge_audit` or `knowledge_log` tool.
- Catalog current state is the special Catalog read
  `kc {verb:"read", flags:{catalog:true}}` (or use the Catalog ID as the
  `catalog` value). Do not include `workspace` or `object`: adding a Workspace
  selects the knowledge-read surface, where `object` is mandatory. `status` is
  local store/process state and `audit` is Catalog history; neither is the
  Catalog's current combination state.
- On a first-time provider flow, `knowledge_context` may report
  `state:"uninitialized"`. That is already the first-run check: proceed to
  `init` instead of probing `read` or `status` against the missing home.
- A ready `knowledge_context` reports `capabilities.search`. When it is `false`,
  do not probe `knowledge_search`; call `knowledge_list` once without
  `objectPrefix`, then read exact objects. An empty prefix is equivalent to an
  omitted prefix. Copy any later prefix from an observed `objectId` rather than
  guessing `table/`, `metric/`, or another entity-type path.
- A Binding is a pinned, stable access declaration, not live content. Knowledge
  Serving uses its State runtime for ordinary exact reads; the `resource` tool
  invokes other explicitly declared operations. Neither call updates Canonical
  knowledge; a Collector must publish an explicit COMMIT. Mounted files remain
  the raw declaration view.

If SEARCH reports `CAPABILITY_UNSATISFIED`, the result is not “no matches.” A
local `index: none` profile intentionally has no search projection; browse with
`knowledge_list` or use mounted files with `rg`. Configure the service
OpenSearch provider when SEARCH is required. Do not add a SQLite or in-memory
implementation that behaves differently from production.

The Agent automatically fixes identity, Catalog, Workspace, and the session
pin. The user still supplies intent: the object/query, desired operation, and
any authorization or high-risk confirmation that cannot be derived. Never ask
the user to manually copy these fixed coordinates into consumer tool calls.

When answering a concept question, explain the distinction and its observable
consequence first, then recommend the smallest next action. State unsupported
capabilities and governance boundaries plainly; do not invent a convenience
surface. In particular, there is no cross-Repository atomic write, no following
latest during one task, no direct file/git write around Writer, and no claim
that a knowledge `permissions` Aspect enforces source-system authorization.

A provider publishes to one target Repository through Writer; no Workspace is
required first. A Workspace is defined only when consumers need a composed
view of one or more already registered Repositories.

When a provider task supplies an existing Connector/Collector integration,
treat it as an executable artifact, not a development assignment. Read its
manifest and operator README, then use its declared Adapter/Collector/preview
entry points. Do not load `integration-development`, inspect implementation
source, or run its package tests unless the user asks to modify it or execution
fails. A normal first publication ends after the requested schemas and source
observations are committed, the consumer Workspace resolves, and representative
objects can be read. Do not add permission matrices, unauthorized probes,
audit/log sweeps, repeated reconciliation, or Search checks unless requested.
When `ingest` output will be committed, call it once with `out` set to the
ChangeSet path and always include the target `repo`; do not first call it
without `out` merely to inspect the same input. Ingest a supplied knowledge
root as one unit when it intentionally contains both schemas and objects. A
Workspace `source` is `repository=selector`; append `@mount/path` only when the
user or repository recipe explicitly requires a mount path. Never assign two
sources to the same root `@` mount.

For knowledge consumption, call `knowledge_search` or `knowledge_read` directly.
The host automatically fixes identity, Catalog, Workspace, and one immutable pin
for every `knowledge_*` and `resource` call; do not copy those coordinates into
tool arguments. `knowledge_context` is optional diagnostics when you need to
report or troubleshoot that scope. Files already mounted by `kcfs` use the
normal filesystem and `rg` tools.

When the object ID or searchable fields are unknown:

1. Use `knowledge_search` for natural-language discovery when search is available.
2. Use `knowledge_schema` to obtain exact text/filter/sort field identities; do
   not guess an ambiguous bare field path.
3. If search is unavailable, use one bounded `knowledge_list` without
   `objectPrefix` to browse lightweight summaries. Continue its page when
   necessary with the short task-local continuation handle it returns. Use
   `objectPrefix` only after copying a real prefix from the returned object IDs;
   do not guess paths from entity types.
4. Use `knowledge_relations` only for one-hop canonical relations around a known
   object. It is not a recursive graph search.

If SEARCH reports `CAPABILITY_UNSATISFIED`, do not repeat it with guessed fields.
Use `knowledge_schema`; when this Workspace intentionally has no projection, use
the mounted files with `rg` or browse IDs with `knowledge_list`.

Use the generic `kc` tool only for operator, publisher, reviewer, and recovery
workflows that are not covered by the typed consumer tools. It calls the same
command table as the `kc` CLI and starts a local `kc serve` on first use when
necessary. Its `flags` object uses CLI flag names without `--`; arrays represent
repeated flags. `verb` belongs only at the top level; never repeat or nest it in
`flags`. Never put `as`, `on-behalf-of`, `home`, or `listen` in `flags`:
the plugin fixes identity and process coordinates for this composition.

The only assumed user artifact is an empty working directory. The plugin may
create `.kc-home` there. Ask only for credentials, authorization that cannot be
derived, or confirmation immediately before a genuinely high-risk action.

## Identity boundary

Identity has two mutually exclusive deployment modes:

- Authenticated Gitea mode: the plugin sends a configured user token and
  `kc serve` verifies it with Gitea. The resulting principal is the stable
  `gitea:<numeric-user-id>` returned by `whoami`; neither the model nor request
  flags can select it. Never request, read, print, or place the token in tool
  flags or knowledge.
- Local development mode: `KC_AS` / `X-Kc-As` is an untrusted claimed
  principal used to exercise authorization rules. An empty principal is the
  local workspace owner.

Call `whoami` when the task depends on identity. Never retry a denial under a
different principal, never claim another role, and never fall back to owner.
Gitea administrator status only permits local Catalog administration; ordinary
knowledge operations still require matching KC rules. Every call carries a
unique request ID; preserve a supplied ID when retrying an identical operation.

Treat these as separate sessions/compositions:

- Catalog Owner (empty principal locally; authenticated service admin in Gitea
  mode): initialize, attach/register repositories, define the Workspace, configure grants
  and gates.
- Producer: write or propose only to the granted repository.
- Reviewer/Gatekeeper: preview, validate, record named evidence, and merge only
  when every required gate is satisfied.
- Consumer: use the session context and read/search only through its fixed pin.
- Auditor: inspect Catalog audit plus object log and provenance; do not mutate.
- Unauthorized Actor: expect `FORBIDDEN`; a denial is evidence, not a reason to
  change identity.

## From an empty workspace

Use explicit IDs so retries and evidence are reproducible. A minimal owner
bootstrap is:

1. `kc {verb:"init", flags:{catalog:"kr://acme/catalog"}}`
2. `kc {verb:"repo-add", flags:{repo:"kr://acme/public/core"}}`
3. Seed an initial object with owner `put` if the governed proposal needs a
   prior value. The Repository is mandatory; use one complete call rather than
   discovering missing flags by retrying:

   `kc {verb:"put", flags:{repo:"kr://acme/public/core", object:"policy/P-1", value:{"v":1}, "command-id":"seed-1", "origin-kind":"SOURCE", "source-ref":"agent://owner/bootstrap", "actor-ref":"workspace-owner"}}`
4. `kc {verb:"define-workspace", flags:{workspace:"agent", revision:1,
   source:["kr://acme/public/core=refs/heads/main"]}}`
5. Add one `allow` rule per command family and scope. Do not combine commands
   rejected by the CLI as different write surfaces.

The owner should normally grant:

- Producer: `propose` on the target repository.
- Reviewer: `preview,validate,record-validation` on the Catalog/Workspace as
  separate allowed command-family calls, plus `merge` on the repository.
- Consumer: `read-workspace` on the Catalog/Workspace and `read,list,search,resolve`
  as separate compatible grants when required by the command-family rules.
- Auditor: `audit` on the Catalog plus `log,provenance,read` on the repository.

Use the exact principal returned by the target role's `whoami`. For Workspace
consumption, grant the exact command family:

`kc {verb:"allow", flags:{principal:"consumer", cmd:"read-workspace", catalog:"kr://acme/catalog", workspace:"agent"}}`

Use `allowed` to check the exact principal/verb/scope before handing off.

## Governed publication

Catalog Owner configures a merge gate in one call. `require` is one comma-separated
string, for example:

`kc {verb:"gate-add", flags:{on:"merge", repo:"kr://acme/public/core", require:"validate,suite:approval:steward"}}`

The successful response is the persisted rule. If inspection is needed, use
`gate-ls`; never inspect KC home files or source code to infer policy state.

Producer:

1. Call `propose` once with stable `proposal-id`, target repository, target
   `refs/heads/main`, candidate `refs/heads/candidates/<id>`, Address fields and
   JSON `value`. Use these exact flag names (not `ref`, `provenance`, or
   camelCase):

   `kc {verb:"propose", flags:{"proposal-id":"PR-1", repo:"kr://acme/public/core", target:"refs/heads/main", candidate:"refs/heads/candidates/PR-1", object:"policy/P-1", value:{"v":2}, "origin-kind":"SOURCE", "source-ref":"agent://producer/PR-1", "actor-ref":"producer"}}`

   A successful response containing `proposalId` and `candidateCommit` is
   complete. Provenance is stored but intentionally not echoed in that
   response. Do not reissue the proposal to make provenance appear; inspect it
   after merge with `provenance`.

Reviewer/Gatekeeper:

1. `preview` with the proposal ID and Workspace; the flag is `proposal`, not
   `proposal-id`: `kc {verb:"preview", flags:{proposal:"PR-1", workspace:"agent"}}`.
   Save the returned `previewId`.
2. `validate` the exact preview and require `outcome: PASSED`:
   `kc {verb:"validate", flags:{preview:"<previewId>"}}`.
3. If a named external suite or approval is required, run it outside Catalog,
   then `record-validation` for the same preview. This command records supplied
   evidence; it does not run a suite.
4. When a merge gate exists, call `merge` with the exact proposal and preview
   IDs. KC evaluates all stored validation evidence bound to that Preview; do
   not pass an array of report IDs. Without a configured gate, pass the one
   successful structural or external report as `validation`.
5. A successful merge receipt reports repository, target ref, Preview basis and
   required checks. Treat that public receipt as authoritative; never inspect
   `control.json`, `gates.json`, source code, or the KC home.

For every successful mutation, stop unless the workflow explicitly requires a
different next verb. Inspect with read-only commands; do not resend a mutation
with guessed flags. Unknown flags are not evidence that a field was stored.

After merge, a new command resolves a new Workspace pin. An already running
consumer remains on its old pin and must not silently follow HEAD.

## Consumption and maintenance

- Every typed knowledge and resource call lazily establishes and reuses the same
  task-local pin. DSH task disposal releases that local context; it is not a KC
  `WorkspaceSession` or `sessionId`. Use `knowledge_context` only for identity/scope diagnostics.
- Use `knowledge_read` and `knowledge_search`; use `knowledge_list`,
  `knowledge_schema`, and `knowledge_relations` for discovery. Exact repository maintenance reads
  must name a ref or commit. Index hits only locate candidates; read canonical
  content before answering.
- `audit` is Catalog registry/operation history. `log` is object digest history.
  `provenance` returns origin envelopes and does not crawl `sourceRefs`.
- Auditor calls have distinct operands: `audit` names the Catalog;
  `log` and `provenance` must both name an object and a pinned Repository
  ref/commit. Never call `log` without `object` expecting Repository history.
- On `NON_FAST_FORWARD`, reread the current commit, redo the diff, use a new
  command ID if content changed, and retry. Identical retries keep command ID.
- A Workspace write advances its target repository. When reporting after a
  write, label the earlier resolved pin/commit as the pre-write coordinate and
  use the write receipt or a fresh `resolve` for the post-write coordinate.
  Never present the pre-write commit as the current result.
- On `FORBIDDEN`, stop that role. On missing flags or bad shape, correct the
  request. Never bypass Writer by editing repository files directly.

## Live resources

An Aspect with `value_source.kind: "binding"` is a versioned access declaration,
not the live payload. Use the `resource` tool with its object ID, Aspect name,
one declared operation, and only the declared input. A Binding may reference a
reusable ResourceDescriptor at the same Snapshot pin. Do not extract or invent
an endpoint, credential, runtime identity, or source path.

The DSH composition supplies the user principal, Agent preset, session,
delegation and request identity. The resource runtime records those coordinates
together with the pinned declaration repository/commit/digest and actual
runtime generation. A resource call does not update knowledge; only a Collector
using Writer COMMIT may do that.

Before reporting completion, collect only evidence required by the user's job:

- Ordinary provider onboarding: publication receipts, the resulting Workspace
  pin when a Workspace was requested, and representative reads.
- Ordinary consumption: the requested read/relation/provenance facts and the
  fixed Workspace pin. Do not add audits, permission probes, or maintenance.
- Governed proposal/review or explicit conformance work: include the applicable
  validation/merge, audit/log/provenance, permission, and denial evidence.
