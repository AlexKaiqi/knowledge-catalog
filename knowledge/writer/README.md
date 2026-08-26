# knowledge/writer/

Writer 是② Knowledge 写入面：一次一个 target、一种 Surface、一个 `command_id`。字面路径写入由 `snapshot/treewriter/` 承担。

```text
COMMIT / PROPOSAL  → Knowledge tree codec → snapshot.TreeStore authority
ChangeSet          → PUT / REMOVE Address
```

PUT 替换一个完整 Address 单元，可携带 `schema_ref`、provenance 与 `value_source`。`value_source.kind=binding` 只版本化访问声明；Writer 不调用 runtime，也不写瞬时 state/stream observation。动态值要沉淀为知识时，墙外 Collector 显式翻译为 Snapshot ChangeSet 再 COMMIT。

`Ingest` / `Reconcile` 只产生 ChangeSet 预览，不是采集框架。PROPOSAL 只推进 candidate Ref；ControlPlane merge 才推进发布 Ref。

幂等规则：同 command_id 同 digest 返回原 Receipt（REPLAYED）；同 id 异 digest 是 `IDEMPOTENCY_CONFLICT`。Snapshot CAS 过期是 `NON_FAST_FORWARD`。带 `schema_ref` 的 PUT 必须在 target commit 可解析。DERIVATION 必须携带固定 inputWorkspaceVersionRef 和 algorithm。

主要文件：`writer.go` 共用校验与 applySnapshot；`commit.go`、`propose.go` 两个 Surface；`schema.go` 校验 schema_ref；`idempotency.go` 解释共享 command ledger 中的 Knowledge 请求；`receipt.go` 定义 durable receipt；`preview.go` 只做预览。

```bash
go run ./cmd/kc -- put --command-id sync-1 --repo kr://acme/public/core \
  --object Service:orders --aspect health --value null \
  --value-source '{"kind":"binding","binding":{"mode":"state","runtime":"orders","protocol":"mcp","operations":{"read":{"call":"health.read"}}}}'
```
