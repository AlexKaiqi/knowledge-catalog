# knowledge/writer/

Writer 是② Knowledge 写入面：一次一个 target、一种 Surface、一个 `command_id`。字面路径写入由 `snapshot/treewriter/` 承担。

```text
COMMIT / PROPOSAL  → Knowledge tree codec → snapshot.TreeStore authority
ChangeSet          → PUT / REMOVE Address
```

文件型 authority 的 codec 只读取本次 Operations 涉及的对象 locator 与 unit，并在同一
TreeChangeSet 更新每对象一个的 `.kc/knowledge-locators/objects/*.entry`；不存在单对象提交时
重读/重写全仓 manifest 的稳态路径。旧 whole-repository locator 会失败关闭并要求先做显式
维护入口 `RebuildTreeLocators` 迁移，普通 Writer 请求不会暗中全仓扫描。native provider 直接消费 provider-neutral `unitcodec.Unit`，
该代数没有 path、扩展名或序列化字段。

PUT 替换一个完整 Address 单元，可携带 `schema_ref` 与 provenance。Bound State 写在 Domain Schema `origin` 上，Writer 拒绝实例 `value_source.kind=binding`。动态值要沉淀为知识时，墙外 Collector 显式翻译为 Snapshot ChangeSet 再 COMMIT。

更新未指定 `path_hint` 时保留该 Address 的原有路径；显式指定才迁移。原生 provider 接收原始 ChangeSet，合并现有单元后仅为尚无路径的单元分配默认路径，Writer 不提前把路径省略改写成迁移。

`Ingest` / `Reconcile` 只产生 ChangeSet 预览，不是采集框架，也不是 ingestion control（面 3 只追 published HEAD 的派生投影）。产品 CLI 的目录写入是 `writer commit --dir`；可选 `kc diff --dir` 看同一对照，不把 ChangeSet 交给调用方。HTTP Writer 仍收 ChangeSet。PROPOSAL 只推进 candidate Ref；ControlPlane merge 才推进发布 Ref。人在自有 git 上改 frontmatter 不经过本 CLI 时，进 published 的知识发布仍须过与 COMMIT 相同的校验（CI 代发或只验不写，见 `TASK.md` WRITE-VALIDATE）；本包本轮不新增 validate-only 命令。宿主发布内置 System Schema 使用 `PublishSystem`：只写入空的 `kr://kc/system` Snapshot，已有对象必须与二进制 digest 一致。

幂等规则：同 command_id 同 digest 返回原 Receipt（REPLAYED）；同 id 异 digest 是 `IDEMPOTENCY_CONFLICT`。Snapshot CAS 过期是 `NON_FAST_FORWARD`。带 `schema_ref` 的 PUT 必须在 target commit 可解析。DERIVATION 必须携带固定 inputWorkspaceVersionRef 和 algorithm。

Relation PUT 可以引用其他仓的对象；仅校验 `CanonicalRelation` 的结构化信封，不读取或写入端点仓，不为端点发权。单仓 target、CAS、幂等与失败原子性不变。端点是否存在和是否允许读取留给消费上下文处理。

账本先持久化 `PENDING` 再触碰 authority。提交后回执丢失时不得重放；运维以原 digest
`ResolvePending` 写回已核对回执，或以 `AbandonPending` 明确放弃。`Prune` 只删除超过部署
保留窗口的 `APPLIED`/`ABANDONED`，永不推断 `PENDING`；Bolt 实现游标删除，不在启动时加载
全部历史。Canonical 与账本已接受后，次级 evidence sink 失败不反转写入结果。

每个 `schema/*` PUT 先按 System Meta Schema 归一化；未知逻辑 type、物理 access 词或错误
形状返回 `SCHEMA_UNSUPPORTED`。复用同一 Schema object ID 时先做兼容性 diff；字段删除、
改类型、新增必填、归属/模式变化或约束收紧返回 `SCHEMA_INCOMPATIBLE`，接入方必须发布新
major。`schema/core/readme/v1` 是平台 README Aspect：副本必须与 System 出版物 digest 一致，
实例必须是 Aspect `readme`，正文为 markdown `body`。兼容的文档变化还必须对固定 basis 上全部引用实例成立：Writer 经有界
`SchemaReferrerLocator` 取回引用者并逐个校验，失配返回 `SCHEMA_INSTANCE_INVALID`；同批
PUT/REMOVE 的 Address 由本批结果承担。REMOVE 一个仍被引用的 `schema/*` 返回
`SCHEMA_INCOMPATIBLE`。带 `schema_ref` 的实例 PUT 使用同批 Schema 草稿或目标仓固定 basis
校验；**省略 `schema_ref` 时继承该 Address 已存储的声明并同样校验**，不符合合同返回
`SCHEMA_INSTANCE_INVALID`，且不推进 Ref。Binding PUT 没有内联 Snapshot 值，Writer
只校验其 Schema 引用和稳定声明。

主要文件：`writer.go` 共用校验与 applySnapshot；`commit.go`、`propose.go` 两个 Surface；`schema.go` 校验 Meta Schema、schema_ref 和实例；`idempotency.go` 解释共享 command ledger 中的 Knowledge 请求；`receipt.go` 定义 durable receipt；`preview.go` 只做预览。

```bash
go run ./cmd/kc -- writer put --command-id schema-1 --repo kr://acme/public/core \
  --object schema/service.health \
  --value '{"entity":"Service","aspect":"health","origin":"https://stats.example","fields":{"status":{"type":"string"}}}'
```
