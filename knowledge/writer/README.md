# knowledge/writer/

Writer 是② Knowledge 写入面：一次一个 target、一种 Surface、一个 `command_id`。字面路径写入由 `snapshot/treewriter/` 承担。

```text
COMMIT / PROPOSAL  → Knowledge tree codec → snapshot.TreeStore authority
ChangeSet          → PUT / REMOVE Address
```

PUT 替换一个完整 Address 单元，可携带 `schema_ref`、provenance 与 `value_source`。`value_source.kind=binding` 只版本化访问声明；Writer 不调用 runtime，也不写瞬时 state/stream observation。动态值要沉淀为知识时，墙外 Collector 显式翻译为 Snapshot ChangeSet 再 COMMIT。

`Ingest` / `Reconcile` 只产生 ChangeSet 预览，不是采集框架。PROPOSAL 只推进 candidate Ref；ControlPlane merge 才推进发布 Ref。

幂等规则：同 command_id 同 digest 返回原 Receipt（REPLAYED）；同 id 异 digest 是 `IDEMPOTENCY_CONFLICT`。Snapshot CAS 过期是 `NON_FAST_FORWARD`。带 `schema_ref` 的 PUT 必须在 target commit 可解析。DERIVATION 必须携带固定 inputWorkspaceVersionRef 和 algorithm。

每个 `schema/*` PUT 先按 System Meta Schema 归一化；未知逻辑 type、物理 access 词或错误
形状返回 `SCHEMA_UNSUPPORTED`。复用同一 Schema object ID 时先做兼容性 diff；字段删除、
改类型、新增必填、归属/模式变化或约束收紧返回 `SCHEMA_INCOMPATIBLE`，接入方必须发布新
major。兼容的文档变化还必须对固定 basis 上全部引用实例成立：Writer 经有界
`SchemaReferrerLocator` 取回引用者并逐个校验，失配返回 `SCHEMA_INSTANCE_INVALID`；同批
PUT/REMOVE 的 Address 由本批结果承担。REMOVE 一个仍被引用的 `schema/*` 返回
`SCHEMA_INCOMPATIBLE`。带 `schema_ref` 的实例 PUT 使用同批 Schema 草稿或目标仓固定 basis
校验；**省略 `schema_ref` 时继承该 Address 已存储的声明并同样校验**，不符合合同返回
`SCHEMA_INSTANCE_INVALID`，且不推进 Ref。Binding PUT 没有内联 Snapshot 值，Writer
只校验其 Schema 引用和稳定声明。

主要文件：`writer.go` 共用校验与 applySnapshot；`commit.go`、`propose.go` 两个 Surface；`schema.go` 校验 Meta Schema、schema_ref 和实例；`idempotency.go` 解释共享 command ledger 中的 Knowledge 请求；`receipt.go` 定义 durable receipt；`preview.go` 只做预览。

```bash
go run ./cmd/kc -- writer put --command-id sync-1 --repo kr://acme/public/core \
  --object Service:orders --aspect health --value null \
  --value-source '{"kind":"binding","binding":{"mode":"state","runtime":"orders","protocol":"mcp","operations":{"read":{"call":"health.read"}}}}'
```
