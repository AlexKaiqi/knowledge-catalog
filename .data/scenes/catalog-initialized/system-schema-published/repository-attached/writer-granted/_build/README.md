# writer-granted

接入方已持有 `writer.commit` / `writer.preview`。空 allow 时 `--as` 写入 `FORBIDDEN`，七列不动。配方 `repo-add` 不发这条权。

构建与探：`cli/write_flow_test.go`、`TestIngestDoesNotProbeExistingSchema`、`TestAuditTrail`。
