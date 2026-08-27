# MySQL TPC-H Connector operator contract

This directory is an already implemented integration fixture. Operate it through
the commands declared in `connector.yaml`. Do not inspect the Python or Go
implementation, run its development tests, inspect container credentials, or
rewrite generated JSON unless one of these declared commands fails.

All source credentials and executable coordinates are already provided through
`KC_MYSQL_CONTAINER`, `KC_MYSQL_PASSWORD`, `PYTHON`, and `CONNECTOR_PREVIEW`.
Pass those environment values through unchanged; never hard-code or print them.
Host `bash` expands `$FIXTURE`; the DSH `kc` tool does not expand shell
variables in JSON flags. For every `kc ingest` `dir`, copy the complete absolute
fixture path from the user's prompt without shortening or reconstructing it.

## Publication sequence

1. Publish the physical Aspect Schemas by ingesting `$FIXTURE/knowledge/physical`
   once with an `--out` ChangeSet, then commit that file to `kr://dw/physical`.
2. Capture current MySQL state once:

   ```bash
   printf '%s\n' '{"checkpoint":{},"signal":{"kind":"bootstrap-full"}}' |
     "$PYTHON" "$FIXTURE/connector/collector.py" > "$KC_WORKSPACE_DIR/mysql.observation.json"
   ```

3. Run the supplied preview against the physical Schema commit:

   ```bash
   "$CONNECTOR_PREVIEW" \
     --manifest "$FIXTURE/connector/connector.yaml" \
     --observation "$KC_WORKSPACE_DIR/mysql.observation.json" \
     --base PHYSICAL_SCHEMA_COMMIT \
     --out "$KC_WORKSPACE_DIR/mysql.preview.json"
   jq '.changeSet' "$KC_WORKSPACE_DIR/mysql.preview.json" \
     > "$KC_WORKSPACE_DIR/mysql.changeset.json"
   ```

4. Commit `mysql.changeset.json` to the physical Repository.
5. Publish all semantic schemas and objects together by ingesting
   `$FIXTURE/knowledge/semantic` once with `--out`, then commit that ChangeSet to
   `kr://dw/semantic`.
6. Define and resolve the requested consumer Workspace, then verify only the
   representative objects needed by the user's request. For multiple
   Repositories use `repository=refs/heads/main` sources without a trailing
   root-mount `@`. If the initial `knowledge_context` was uninitialized, call
   it again after resolve, then discover representative IDs with one unfiltered
   `knowledge_list`. Do not inspect KC home files, test cases, or prior run
   evidence to discover objects.

`model-research.md` is design evidence, not an execution input. A normal first
publication does not require a repeated reconcile, permission matrix, audit/log
sweep, Search probe, raw `kc` CLI fallback, or source-code inspection.

## Adapter diagnostics

Use these only when a declared publication command fails or the user explicitly
asks to inspect the source:

```bash
printf '%s\n' '{"operation":"listTables","arguments":{}}' |
  "$PYTHON" "$FIXTURE/connector/adapter.py"
printf '%s\n' '{"operation":"describeSchema","arguments":{"table":"lineitem"}}' |
  "$PYTHON" "$FIXTURE/connector/adapter.py"
```
