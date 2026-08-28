---
name: knowledge-catalog
description: Explain or operate Knowledge Catalog through the grouped kc CLI and mounted workspace files.
---

# Knowledge Catalog

Do the minimum work required by the user. Use the host shell to invoke the
grouped `kc` CLI; this integration does not provide Knowledge Catalog model
tools.

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

- Known object: `kc knowledge read`.
- Natural-language discovery: `kc knowledge search`; use
  `kc knowledge schema describe` for exact filter/sort fields.
- Relations or origin: `kc knowledge relations` or `kc knowledge provenance`.
- Publishing: `kc writer ...`; governance: `kc governance ...`.
- Catalog and Workspace management: `kc catalog ...`.
- Mounted knowledge files: ordinary `ls`, `find`, `rg`, and `cat`. These
  directories are read-only; unrelated paths in the user working directory are writable.

There is no public Knowledge LIST and no SEARCH-to-scan fallback. If SEARCH is
unavailable, report the capability gap. `rg` over mounted files is file search,
not a claim of complete structured knowledge discovery.

The host supplies identity, Catalog, Workspace and a fixed task pin. Do not
override them or re-resolve in the middle of a task. Use `kc help <group>` when
exact flags are not already supplied.

## Existing provider integration

Treat an existing Connector/Collector as an executable artifact:

1. Read its manifest and operator README.
2. Run the declared Adapter, Collector and preview commands. Do not inspect
   implementation unless execution fails or the user asks to change it.
3. Publish Schema inputs with `kc writer ingest`, then `kc writer commit`.
4. Commit the Connector preview ChangeSet to its target Repository.
5. Define a Workspace only when requested; add a mount path only for an explicit mount.
6. Resolve once and verify only representative objects needed by the request.

If the user names a target Catalog, pass it explicitly to registration,
Workspace definition and resolve. Local authority attachment is
`kc local repository attach`; Catalog recognition is
`kc catalog repository register`. They are different actions.

## Invariants

- Never write Repository files or git directly around Writer.
- Never change identity or retry `FORBIDDEN` as another principal.
- Never follow `latest` after the task pin is established.
- Do not repeat a successful mutation just to inspect it.
- On `NON_FAST_FORWARD`, reread, redo the diff and retry with correct idempotency semantics.
- Catalog audit, object log and provenance are different evidence.
- A permissions Aspect does not enforce source-system access.
- Proposal flow is create proposal → create/validate Preview → record external
  validation when needed → merge.
- Report only evidence required by the user's task.
