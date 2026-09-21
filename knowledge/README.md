# knowledge/

② Knowledge protocol：`object_id`、Address、Aspect、Schema、provenance、Binding，以及 PUT/REMOVE、READ/RESOLVE/LOG/DIFF。

本包直接拥有 `ObjectID`、`Address`、`KnowledgeRef`、Schema ref 与 provenance 类型；它们不是为了少一个 import 而放进 `kernel/` 的“共享类型”。这让 ⓪ Snapshot 和 ① Catalog 在类型层面也无法携带知识语义。

`Repository` 是只读②视图。Gitea 由 `knowledge/reader` 在 Snapshot `TreeStore` 之上解释；规模化 Dolt 由 `knowledge/dolt` 直接解释原生 unit/object 表。两者都只提供 Canonical 精确读，Relation 候选必须来自③ exact-basis Retriever。挂载与 Catalog 仍只要求 `snapshot.Store`，由应用装配显式取得② capability。

`Repository` 只提供精确读、历史与分页，不提供 `Search/Probe/Retrieve`。PUT/REMOVE 只进入 Writer；支持 `knowledge.ChangeStore` 的② provider 可增量落行，否则 Writer 使用字面 tree codec。Snapshot adapters 不解释知识或复制检索逻辑。

`CanonicalRelation` 是独立 Relation Address 的值：至少两个带角色的结构化 KnowledgeRef 端点，不能重复相同角色与引用。端点可以跨仓，Writer 只校验信封并写入关系所在仓，不访问端点 authority。引用不授予读权，也不改变 Schema 同仓解析。关系存储坐标与端点引用坐标分别保留；一跳候选由③查询 Dataset 内的关系存储仓，再按同一 basis 回读。

`Hydrator` 是可替换的固定版本正文读取端口，公开 `ReadMany` 与 `ReadAddress`；它保留 Repository、
commit、完整对象或 Address 的读取形状，且只交付 Snapshot 声明。上层可注入缓存，② Reader
不持有其实现或生命周期。`ValidateHydratedObject` / `ValidateHydratedAddress` 统一校验返回值的
KnowledgeRef、版本、对象及 Units/Declarations 所属；动态 State hydrate 与当前权限交付在此之后。

`kr://kc/system` 是应用根挂载的内置只读 `SystemRepository`，发布
`schema/meta/schema-definition/v1` 和核心协议 Schema。跟踪源是
`system/schemas/`（`go:embed`），与 Canonical 仓内平铺的 `_schemas/` 树一致；
身份仍是 `schema/*` object_id。
README 是 Markdown 知识单元，不是 Catalog 库存的 title/summary。协议 Schema 是
`schema/core/readme/v1`（Aspect `readme`，字段 `body` 声明 `text`）。人写
`README.md`：frontmatter 用 `entity` / `aspect` 承载 Address，正文是 markdown。
身份仍是 Address；`README.md` 只是人写与 git 直推解释用的有界 `path_hint`。
无 frontmatter 的根 README 不是知识。接入方须在同一仓
发布与 System 出版物 digest 一致的 Schema 副本（`schema_ref` 不跨仓）。
领域分类、owner、质量门槛和投影热状态不进入该对象。`catalog/` DTO 与 `kc show`
的 `repositories` 只列 `{id, schemaCount?}`；读 README 走 `kc read --aspect readme`，
默认 SEARCH 编 `body` 的 `text`。
`ParseSchemaDefinition` 与 `ValidateSchemaInstance` 是 Writer/Reader 共用的协议解释。
Schema 文档可有可选 `description`，说明该实体是什么；不是固定元信息，不是仓 README，
也不是 Catalog title/summary。`schemas:list` 可带上这一句。
Meta Schema 不向实例继承字段；Entity / Aspect / Relation / Member 是 Address 种类，
不是 System 仓里要「也列出来」的四种实体。
System Repository 中的可读对象与二进制 canonical digest 必须一致。宿主可以用
`kc deployment system publish --config deployment.yaml` 把同一份对象写入配置绑定的空 Dolt/Gitea Snapshot；已占用仓只校验、不覆盖。Domain Schema 文档的
JSON Schema 词表在 `schema-document.schema.yaml`，只用于对账，不替代 Go 校验器。
时间标量包括 `date`（日历日期）以及 `datetime` / `timestamp`（带时区的 RFC3339 时间）；
实例验证保留 Canonical 的原始字符串、时区与精度，检索层再按逻辑类型正规化。
时间精度为纳秒；小数第九位以后只能是等价的补零，不能静默截断有效精度。
`text/filter/sort` 用于已定义的标量字段，包括 `object_ref_list` 的多值字符串；
`object/record/array/relation_endpoint_list` 保留为可读取的复合知识值，不能直接声明
未定义的标量访问。`keyed_collection` 中各 Member 的标量字段仍可声明访问。
`BreakingSchemaChanges` 约束同一 Domain Schema object ID
只能做兼容演进；单仓 Schema 发现由应用层 `schemas:list` 点名已发布实体，不是 Schema 正文或合同摘要。

Address/pattern、必填与 `additionalProperties` 对每个 `schema/*` 无条件生效；省略
`metaSchema` 只是省略文档声明，不是跳过校验的开关。可选 `SchemaReferrerLocator` 是有界反向
`schema_ref` 索引，供 Schema 发布证明既有实例仍然合法；它必须由版本化索引在同一 basis 回答，
不得退化为 Snapshot 扫描。

`knowledge/writer` 接收 Knowledge `ChangeSet`；⓪ `snapshot.Store` 不接收 PUT/REMOVE。Tree provider 在唯一的②→⓪接缝上编译为 `TreeChangeSet`；`knowledge/dolt` 在②层直接实现 bounded row mutation。两条路径共享 `knowledge/unitcodec` 的 apply/assemble 语义，并由差分 conformance 约束。

批量草稿可直接写成 `*.yaml`、`*.aspect.yaml` 或 `README.md`：frontmatter 声明
Address（`object_id`/`aspect_name`，Markdown 也接受 `entity`/`aspect`）与
`schema_ref`。YAML/JSON 正文是结构化值；README 正文是 `body` 字符串。一个文件
对应一个 Address；`kc writer commit --dir` 对照当前版本求差后进入 Writer。`kc diff --dir` 用同一对照，不写仓。已接入的 tree 仓也可以对 published
ref 直推：Reader 按文件 frontmatter 解释，live 投影对 HEAD 对账。格式转换不得
夹带 source key 映射或领域建模逻辑。HTTP Writer 仍收 ChangeSet。
`schema/*` 默认写入仓内唯一的 `_schemas/` 目录；实例按 `schema_ref` 对应的实体类型
分目录（`metrics/`、`tables/`），不再使用 `objects/` 前缀。身份仍是
`object_id`，目录只是 `path_hint`。

Binding 声明在 Domain Schema Canonical `origin` 上；运行状态、cursor、watermark 和凭证留在墙外 Materialization Runtime。`kc access` 用 origin + 实体 `object_id` 取回该 Aspect，不另存空实例。ResourceDescriptor `origin` 给 `kc invoke`。消费侧 `knowledge/serving` 可通过注入的 State 端口返回观察值及 observation basis，但不会把它写回 Repository 或伪装成 commit 中的值。

目录内聚合整个②垂直面：根包拥有协议类型与只读 Repository 视图，`reader/` 负责固定 commit 上的知识解释，`writer/` 负责 Knowledge ChangeSet 的 COMMIT/PROPOSAL。字面路径写入不在本目录，见 `snapshot/treewriter/`；检索合同不在本目录，见 `retrieval/`。

`semanticview/` 只把已经组装的固定版本 `KnowledgeValue` 渲染成带 `_kc` 坐标的消费 YAML；
它不枚举 Repository、不写回，也不拥有投影生命周期。
