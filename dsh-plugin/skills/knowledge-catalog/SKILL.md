---
name: knowledge-catalog
description: Explain or operate Knowledge Catalog publishing, composition, discovery, reading, governance, and recovery.
---

# Knowledge Catalog

Do the minimum work required by the user.

## Model

- Repository: versioned knowledge authority and write boundary.
- Catalog: registers Repositories and Workspace recipes; it stores no knowledge.
- Workspace: composes Repository selectors without copying knowledge.
- ResolvedWorkspace/pin: one immutable `{repository -> commit}` basis per task.
- `object_id`: stable knowledge identity, not a path or source key.
- Schema: a versioned `schema/*` knowledge object.
- Binding: a stable access declaration, not live content. Only an explicit
  Collector COMMIT changes knowledge.

## Choose the surface

- Known object: `knowledge_read`.
- Natural-language discovery: `knowledge_search`; use `knowledge_schema` for
  exact filter/sort fields.
- Canonical enumeration: bounded `knowledge_list`. It is not a scalable search
  substitute. Never scan every page to emulate search.
- Known endpoint relations or origin: `knowledge_relations` or
  `knowledge_provenance`.
- Operator/write/governance work: `kc`.
- Mounted `kcfs` files: read-only shell projection, not another authority.

`knowledge_context` reports the fixed identity, Workspace, pin and Search
capability. If Search is unavailable, do not call it. A bounded list page may
help in a small catalog; otherwise report that discovery is unavailable.

The host supplies identity, Catalog, Workspace and pin to typed tools. Do not
copy or override them. Use `kc help provider|consumer|governor` when exact
operator flags are not already supplied by the integration contract.

## Existing provider integration

Treat an existing Connector/Collector as an executable artifact:

1. Read its manifest and operator README.
2. Run the declared Adapter, Collector and preview commands. Do not inspect
   implementation or load `integration-development` unless execution fails or
   the user asks to change it.
3. Publish Schema inputs as knowledge. Run `ingest` once with both `repo` and
   `out`, then `commit` that ChangeSet.
4. Commit the Connector preview ChangeSet to its target Repository.
5. Define a Workspace only when requested. A source is
   `repository=selector`; add `@mount/path` only for an explicit mount.
6. Resolve once and verify only representative objects needed by the request.

If `knowledge_context` is `uninitialized`, initialize the Catalog and add the
required Repositories before publishing; call it again only after defining the
Workspace. Do not add permissions, audits, Search probes, repeated reconciliation
or unrelated validation unless asked.

## Invariants

- Never write Repository files or git directly around Writer.
- Never change identity or retry `FORBIDDEN` as another principal.
- Never follow `latest` after the task pin is established.
- Do not repeat a successful mutation just to inspect it.
- On `NON_FAST_FORWARD`, reread, redo the diff and retry with correct
  idempotency semantics.
- `audit` is Catalog history; `log` is object revision history;
  `provenance` is recorded origin.
- A permissions Aspect does not enforce source-system access.
- Proposal flow is `propose -> preview -> validate/evidence -> merge`; use KC
  help for the exact governed workflow.
- Report only evidence required by the user's task.
