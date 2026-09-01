# knowledge/

② Knowledge protocol：`object_id`、Address、Aspect、Schema、provenance、Binding，以及 PUT/REMOVE、READ/RESOLVE/LOG/DIFF。

本包直接拥有 `ObjectID`、`Address`、`KnowledgeRef`、Schema ref 与 provenance 类型；它们不是为了少一个 import 而放进 `kernel/` 的“共享类型”。这让 ⓪ Snapshot 和 ① Catalog 在类型层面也无法携带知识语义。

`Repository` 是只读②视图。Gitea 由 `knowledge/reader` 在 Snapshot `TreeStore` 之上解释；规模化 Dolt 由 `knowledge/dolt` 直接解释原生 unit/object 表。两者都只提供 Canonical 精确读，Relation 候选必须来自③ exact-basis Retriever。挂载与 Catalog 仍只要求 `snapshot.Store`，由应用装配显式取得② capability。

`Repository` 只提供精确读、历史与分页，不提供 `Search/Probe/Retrieve`。PUT/REMOVE 只进入 Writer；支持 `knowledge.ChangeStore` 的② provider 可增量落行，否则 Writer 使用字面 tree codec。Snapshot adapters 不解释知识或复制检索逻辑。

`kr://kc/system` 是应用根挂载的内置只读 `SystemRepository`，发布
`schema/meta/schema-definition/v1` 和核心协议 Schema。跟踪源是 `system/*.yaml`（`go:embed`）；
`ParseSchemaDefinition` 与 `ValidateSchemaInstance` 是 Writer/Reader 共用的协议解释。
System Repository 中的可读对象与二进制 canonical digest 必须一致。宿主可以用
`kc local system publish` 把同一份对象写入空的 Dolt/Gitea Snapshot；已占用仓只校验、不覆盖。Domain Schema 文档的
JSON Schema 词表在 `schema-document.schema.yaml`，只用于对账，不替代 Go 校验器。
`BreakingSchemaChanges` 约束同一 Domain Schema object ID
只能做兼容演进；单仓 Schema 发现由应用层 `schemas:page` 在固定 commit 上分页。

Address/pattern、必填与 `additionalProperties` 对每个 `schema/*` 无条件生效；省略
`metaSchema` 只是省略文档声明，不是跳过校验的开关。可选 `SchemaReferrerLocator` 是有界反向
`schema_ref` 索引，供 Schema 发布证明既有实例仍然合法；它必须由版本化索引在同一 basis 回答，
不得退化为 Snapshot 扫描。

`knowledge/writer` 接收 Knowledge `ChangeSet`；⓪ `snapshot.Store` 不接收 PUT/REMOVE。Tree provider 在唯一的②→⓪接缝上编译为 `TreeChangeSet`；`knowledge/dolt` 在②层直接实现 bounded row mutation。两条路径共享 `knowledge/unitcodec` 的 apply/assemble 语义，并由差分 conformance 约束。

批量草稿可直接写成 `*.okf` 或 `*.aspect.yaml`：frontmatter 声明 Address
身份与 `schema_ref`，正文使用 JSON 或结构化 YAML。一个文件对应一个 Address；
`kc ingest --dir` 只把这些单元机械预览为 ChangeSet，确认后仍由 `kc commit
--changeset` 进入 Writer。格式转换不得夹带 source key 映射或领域建模逻辑。

Binding 在 Repository 中只保存稳定声明；运行状态、cursor、watermark、凭证和实际 endpoint 留在墙外 Materialization Runtime。消费侧 `knowledge/serving` 可通过注入的 State 端口返回绑定后的逻辑值及 observation basis，但不会把它写回 Repository 或伪装成 commit 中的值。

目录内聚合整个②垂直面：根包拥有协议类型与只读 Repository 视图，`reader/` 负责固定 commit 上的知识解释，`writer/` 负责 Knowledge ChangeSet 的 COMMIT/PROPOSAL。字面路径写入不在本目录，见 `snapshot/treewriter/`；检索合同不在本目录，见 `retrieval/`。

`semanticview/` 只把已经组装的固定版本 `KnowledgeValue` 渲染成带 `_kc` 坐标的消费 YAML；
它不枚举 Repository、不写回，也不拥有投影生命周期。
