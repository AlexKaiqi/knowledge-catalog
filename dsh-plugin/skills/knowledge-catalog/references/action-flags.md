# Action flag contracts

Use CLI flag names verbatim as keys in `flags`. Include Catalog/Workspace or
Repository scope on the first call so authorization evaluates the intended
surface. These are the non-obvious shapes used by the operator workflows.

## Ingest and direct writes

- Preview a directory: `ingest {dir, repo, out}`.
- Apply its ChangeSet: `commit {changeset, repo, command-id}`.
- Inspect idempotency: `receipt {command-id}`.
- Append an event: `append {repo, stream, event-id, value, command-id}`.
- Workspace bytes: `vfs-write {catalog, workspace, path, content, command-id}`;
  `content` is base64. Do not use `content-base64`.

## Review gate

- `preview {proposal, catalog, workspace}`; use `proposal`, not `proposal-id`.
- `validate {preview, catalog, workspace}`.
- `record-validation {preview, suite, outcome, catalog, workspace}`.
- `merge {proposal, preview, validation:[...], repo, catalog, workspace}`.
  Pass every report ID required by the gate.

## Workspace consumption

- Resolve and all Workspace reads: include `{catalog, workspace}`.
- Search filters use `eq:"path=value"`; text search uses
  `match:"path=text"`, for example `match:"note=browser"`.
- Stream: `stream {catalog, workspace, stream}`.
- Checkout: `checkout {catalog, workspace, to}`. Advance that checkout with
  `sync {catalog, workspace, to}`. `path` and `dir` are not checkout targets.
- VFS: `vfs-list {catalog, workspace, path}` and
  `vfs-read {catalog, workspace, path}`.

## Hooks, gates, permissions, lifecycle

- Hooks only accept mutating verbs. Use `hook-add {on, phase, run}`; never
  configure a hook on `read`.
- `gate-add` requirements are repeated values; list and remove by returned ID.
- Grant the exact action and scope before handoff. `read-workspace` does not
  imply `index-plan`, and reviewer gate actions do not imply post-merge read.
- `revoke`, `hook-rm`, and `gate-rm` use the exact returned `id`.
- `index-plan`, `retire-workspace`, and `archive-catalog` name Catalog and
  Workspace where applicable; `archive-repo` names the Repository.
