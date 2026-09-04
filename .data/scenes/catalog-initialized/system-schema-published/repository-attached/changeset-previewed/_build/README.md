# changeset-previewed

墙外 Collector 对账后产生 ChangeSet（`connector.Preview`），确认后才 Writer COMMIT。这不是 `kc connector-run`，也不是把源客户端登记进 Catalog。

构建与探：`connector/preview_test.go`、`TestPreviewThenCommit`。
