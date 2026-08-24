---
name: knowledge-catalog
description: Operate a Knowledge Catalog from an empty workspace, including repositories, workspaces, governed publishing, role-scoped access, reads/search, audit, provenance, diagnosis, and recovery.
---

# Knowledge Catalog operator

Use the `kc` tool for Knowledge Catalog operations. It calls the same command
table as the `kc` CLI and starts a local `kc serve` on first use when necessary.
The `flags` object uses CLI flag names without `--`; arrays represent repeated
flags. Never put `as`, `home`, or `listen` in `flags`: the plugin fixes the
actor and service coordinates for this agent composition.

Do not issue an empty-flags probe to discover a command shape. Authorization
is scope-sensitive, so an empty probe can look like a real `FORBIDDEN`. For
ingest, review gates, search, checkout/VFS, hooks, and lifecycle actions, read
[references/action-flags.md](references/action-flags.md) before the first call.

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
  mode): initialize, mount repositories, define the Workspace, configure grants
  and gates.
- Producer: write or propose only to the granted repository.
- Reviewer/Gatekeeper: preview, validate, record named evidence, and merge only
  when every required gate is satisfied.
- Consumer: resolve the Workspace once, then read/search only through that pin.
- Auditor: inspect Catalog audit plus object log and provenance; do not mutate.
- Unauthorized Actor: expect `FORBIDDEN`; a denial is evidence, not a reason to
  change identity.

## From an empty workspace

Use explicit IDs so retries and evidence are reproducible. A minimal owner
bootstrap is:

1. `kc {verb:"init", flags:{catalog:"kr://acme/catalog"}}`
2. `kc {verb:"repo-add", flags:{repo:"kr://acme/public/core"}}`
3. Seed an initial object with owner `put` if the governed proposal needs a
   prior value.
4. `kc {verb:"define-workspace", flags:{workspace:"agent", revision:1,
   source:["kr://acme/public/core=refs/heads/main@"]}}`
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

1. `preview` with the proposal ID and Workspace; save `previewId`.
2. `validate` the exact preview and require `outcome: PASSED`.
3. If a named external suite or approval is required, run it outside Catalog,
   then `record-validation` for the same preview. This command records supplied
   evidence; it does not run a suite.
4. `merge` using the exact proposal, preview, and validation report IDs. Never
   omit failed or missing gate evidence.

For every successful mutation, stop unless the workflow explicitly requires a
different next verb. Inspect with read-only commands; do not resend a mutation
with guessed flags. Unknown flags are not evidence that a field was stored.

After merge, a new command resolves a new Workspace pin. An already running
consumer remains on its old pin and must not silently follow HEAD.

The Loom filesystem pin is established when the DSH host composition starts.
If a `kc` mutation in that composition advances a mounted Ref, filesystem
Write/Edit may correctly return `NON_FAST_FORWARD`. Do not retry in a loop or
bypass it with host files. Finish the governed mutation, restart the DSH host
with the same fixed principal, and continue in a fresh session.

## Consumption and maintenance

- Start consumption with `resolve {workspace:"..."}` and keep the returned
  `{repository -> commit, appendCuts}` coordinate fixed for that task.
- Use Workspace `read`, `list`, and `search`; exact repository maintenance reads
  must name a ref or commit. Index hits only locate candidates; read canonical
  content before answering.
- `audit` is Catalog registry/operation history. `log` is object digest history.
  `provenance` returns origin envelopes and does not crawl `sourceRefs`.
- On `NON_FAST_FORWARD`, reread the current commit, redo the diff, use a new
  command ID if content changed, and retry. Identical retries keep command ID.
- A Workspace write advances its target repository. When reporting after a
  write, label the earlier resolved pin/commit as the pre-write coordinate and
  use the write receipt or a fresh `resolve` for the post-write coordinate.
  Never present the pre-write commit as the current result.
- On `FORBIDDEN`, stop that role. On missing flags or bad shape, correct the
  request. Never bypass Writer by editing repository files directly.

Before reporting completion, show the relevant pin/commit IDs, validation and
merge evidence, consumer read/search result, audit/log/provenance evidence, and
one unauthorized operation that failed with `FORBIDDEN`.
