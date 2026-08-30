---
name: knowledge-catalog
description: Explain or operate Knowledge Catalog through the grouped kc CLI and mounted workspace files.
---

# Knowledge Catalog

Do only the user's task. Invoke grouped `kc` CLI through the host shell; this
integration has no Knowledge Catalog model tools.

## First contact

The user does not need to know KC commands. For a new user, say this task has a
read-only, fixed-version knowledge Workspace and accepts natural language.
Explicitly contrast: the host supplies identity, Catalog, Workspace and task
pin; the user supplies only a topic/object, scope and desired output. Never ask
them to configure those coordinates for a knowledge question.

For the first useful result:

1. Derive a focused query from the user's words and run `kc knowledge search`.
2. Read the most relevant CandidateRef with `kc knowledge read`; do not present
   a search hit as Canonical content.
3. Answer in user language. Mention the fixed basis or provenance only when it
   helps the request or the user asks.

If asked what exists, do not invent LIST. Ask for a topic or offer focused
searches. The sidebar “知识” is read-only browsing, not complete discovery.

## Model

- Repository: versioned knowledge authority and write boundary.
- Catalog: registers Repositories and Workspace recipes; it stores no knowledge.
- Workspace: composes Repository selectors without copying knowledge.
- ResolvedWorkspace/pin: one immutable `{repository -> commit}` basis per task.
- `object_id`: stable knowledge identity, not a path or source key.
- Source keys and the mapping from source-system identity to `object_id` belong
  to the provider/integration side. They are not Catalog state, Binding or
  provenance; provenance records the published object's source envelope but
  does not replace the provider's identity mapping.
- Schema: a versioned `schema/*` knowledge object.
- Binding: a stable access declaration, not live content. Only an explicit
  Collector COMMIT changes knowledge.

## Choose the surface

- Known object: `kc knowledge read`.
- Natural-language discovery: `kc knowledge search`; use
  `kc knowledge schema describe` for exact filter/sort fields.
- Relations or origin: `kc knowledge relations` or `kc knowledge provenance`.
- Known ResourceDescriptor operation: `kc resource access --object <id>
  --operation <name> --input <json>`. Operation/call come from the pinned
  declaration. Read the descriptor with `knowledge read`; `resource read` does
  not exist. Never call its runtime URL or infer live results from files.
- Publishing: `kc writer ...`; governance: `kc governance ...`.
- Catalog and Workspace management: `kc catalog ...`.
- Mounted knowledge files: ordinary `ls`, `find`, `rg`, and `cat`. These
  directories are read-only; unrelated paths in the user working directory are writable.

SEARCH `CAPABILITY_UNSATISFIED` means `index:none`/no provider, not no match.
Configure OpenSearch; never invent SQLite/memory. There is no public Knowledge LIST
or scan fallback. `rg` is mounted-file search, not structured discovery.

## Existing provider integration

Treat an existing Connector/Collector as executable:

1. Read its manifest and operator README.
2. Run the declared Adapter, Collector and preview commands. Do not inspect
   implementation unless execution fails or the user asks to change it.
3. Publish Schema inputs with `kc writer ingest`, then `kc writer commit`.
4. Commit the Connector preview ChangeSet to its target Repository.
5. Define a Workspace only when requested; add a mount path only for an explicit mount.
6. Resolve once and verify only objects needed by the request.

Publishing targets a Repository and does not require a Workspace first. Define
a Workspace only when a user needs composition, consumption or an explicit
mount. A provider may keep draft/schema fixtures as inputs, but the Canonical
Schema is the versioned `schema/*` object published through Writer, not a
project source file.

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
