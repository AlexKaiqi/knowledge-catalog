# MySQL TPC-H Connector operator contract

This directory is an already implemented integration fixture. Operate it through
the commands declared in `connector.yaml`. Do not inspect the Python or Go
implementation, run its development tests, inspect container credentials, or
rewrite generated JSON unless one of these declared commands fails.

All fixture access and executable coordinates are already provided through
`KC_MYSQL_CONTAINER`, `KC_MYSQL_AUTH`, `PYTHON`, and `CONNECTOR_PREVIEW`.
Pass those environment values through unchanged; never hard-code or print them.
The Agent invokes the grouped `kc` CLI through the host shell. Host `bash`
expands `$FIXTURE`; never put credentials in command output. For every
`kc pack --dir`, copy the complete absolute
fixture path from the user's prompt without shortening or reconstructing it.

## Publication sequence

Use grouped `kc` CLI commands for every KC operation. Initialize
`kr://dw/catalog`, then attach `kr://dw/physical` and `kr://dw/semantic` with
`kc local init` and `kc local repository attach` before
publishing. If help is needed, the exact topic is `provider`.

1. Publish the physical Aspect Schemas by ingesting `$FIXTURE/knowledge/schemas/physical`
   once with an `--out` ChangeSet, then commit that file to `kr://dw/physical`.
   Publish the SQL ResourceDescriptor by ingesting `$FIXTURE/knowledge/physical`
   and committing it. `commit` flags are `repo`, `changeset`, and hyphenated `command-id`.
2. Capture current MySQL state once:

   ```bash
   printf '%s\n' '{"checkpoint":{},"signal":{"kind":"bootstrap-full"}}' |
     KC_MYSQL_PASSWORD="$KC_MYSQL_AUTH" \
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
5. Publish semantic Schema by ingesting `$FIXTURE/knowledge/schemas/semantic`,
   then publish instances by ingesting `$FIXTURE/knowledge/semantic`. Commit each
   ChangeSet to `kr://dw/semantic`.
6. Define the Workspace with `kc workspace define` and flags `catalog`,
   `workspace`, numeric `revision: 1`, and repeated `source`; then `resolve` it and verify
   only the representative objects needed by the user's request. For multiple
   Repositories use `repository=refs/heads/main` sources without a trailing
   root-mount `@`. Resolve once with `kc workspace pin`, retain that
   pin, and discover representative IDs with `kc knowledge search`. If Search
   capability is unavailable, report it explicitly; do not enumerate or scan
   the Repository. Do not inspect KC home files, test cases, or prior run evidence
   to discover objects.

`model-research.md` is design evidence, not an execution input. A normal first
publication does not require a repeated reconcile, permission matrix, audit/log
sweep, Search probe, raw `kc` CLI fallback, or source-code inspection.

## Adapter diagnostics

Use these only when a declared publication command fails or the user explicitly
asks to inspect the source:

```bash
printf '%s\n' '{"operation":"listTables","arguments":{}}' |
  KC_MYSQL_PASSWORD="$KC_MYSQL_AUTH" \
  "$PYTHON" "$FIXTURE/connector/adapter.py"
printf '%s\n' '{"operation":"describeSchema","arguments":{"table":"lineitem"}}' |
  KC_MYSQL_PASSWORD="$KC_MYSQL_AUTH" \
  "$PYTHON" "$FIXTURE/connector/adapter.py"
```

## No-op synchronization preview

When the host provides `KC_DW_CHECKPOINT`, validate that the current source and
published state still agree without writing KC. Run these commands exactly; the
checkpoint is connector runtime state, not Canonical knowledge:

```bash
jq '{checkpoint:.nextCheckpoint,signal:{kind:"explicit-full-reconcile"}}' \
  "$KC_DW_CHECKPOINT" |
  KC_MYSQL_PASSWORD="$KC_MYSQL_AUTH" \
  "$PYTHON" "$FIXTURE/connector/collector.py" \
  > "$RUN/agent-provider.observation.json"

physical_head="$("$KC_BIN" local status |
  jq -r '.repos[] | select(.id == "kr://dw/physical") | .head')"
"$CONNECTOR_PREVIEW" \
  --manifest "$FIXTURE/connector/connector.yaml" \
  --observation "$RUN/agent-provider.observation.json" \
  --base "$physical_head" \
  --out "$RUN/agent-provider.preview.json"

jq '{desired:(.desired|length),observed:(.observed|length)}' \
  "$RUN/agent-provider.observation.json"
jq '{empty,summary}' "$RUN/agent-provider.preview.json"
```

Success means `desired=101`, `observed=101`, `empty=true`, and zero
added/updated/removed operations. A non-empty preview is a change proposal, not
permission to call Writer automatically.
