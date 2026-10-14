# object/

业务对象的基本合同：`object_id`、Address、Aspect、Schema、关系、来源、Binding 纯声明，以及 PUT/REMOVE、固定 READ/RESOLVE/LOG/DIFF。对象的对象表示由 Snapshot 承载；lakeFS 只提供内容与版本，不解释业务对象。

本包直接拥有 `ObjectID`、`Address`、`ObjectRef`、Schema ref 与 provenance 类型；它们不是为了少一个 import 而放进 `kernel/` 的“共享类型”。这让 ⓪ Snapshot 和 ① Dataset 服务 在类型层面也无法携带对象语义。

`Reference` 是统一外部访问目标。`FormatReference` / `ParseReference` 选定形状为
`urn:kc:object:<encoded-repo>:<encoded-object>#<pointer>`：Repo 和 object_id 各自进行 URI
百分号编码，冒号只分隔这两个身份分量；fragment 是 JSON Pointer 的 URI 表示。例：
`urn:kc:object:kr%3A%2F%2Fscene%2Fobject:table%2Forders#/columns/order_id/constraints/nullable`。
fragment 缺省或为空表示整个对象；`~0` 与 `~1` 分别选择键中的 `~` 与 `/`，百分号先解码一次，
Pointer 再解码一次。`PointerTokens` / `SelectPointer` 区分缺失与 JSON null，数组索引仅用于读取。
语法错误返回 `USAGE_INVALID`，不存在的选值返回 `OBJECT_REF_UNRESOLVED`。
Dataset 范围和固定 commit 属于独立请求坐标，不改变 URN 来源身份。

读取可选择任意节点，`Digest` 保留其完整维护单元的依据；Members 集合没有单一维护摘要。
写入由同一个 Reference 选择现有 Entity／Relation、完整 Aspect／Record 或单个 Member；
PUT 完整替换，REMOVE 显式删除。创建首个 Member 集合须声明 `KindMember`，不能仅凭两段
Pointer 猜维护模型。深字段、Members 集合、数组元素与组合对象整体 PUT 不属于写面。
`ReferencePlanner` 只交付同一固定版本的维护坐标，为授权提供依据，不读取正文；
`ReferenceUnitsReader` 交付所选完整单元，供外部逻辑访问在 leaf 选择前应用观察。
基础类型、单元组装与修改语义在本根包唯一维护；`CommitReceipt` 等回执纯类型也在本包，
Writer 兼容别名不另造回执合同。

`Repository` 是只读②视图。Tree provider 由 `object/reader` 在 Snapshot `TreeStore` 之上解释。都只提供 Canonical 精确读，Relation 候选必须来自③ exact-basis Retriever。挂载与 Dataset 服务 仍只要求 `snapshot.Store`，由应用装配显式取得② capability。

`Repository` 只提供精确读、历史与分页，不提供 `Search/Probe/Retrieve`。PUT/REMOVE 只进入 Writer；支持 `object.ChangeStore` 的② provider 可增量落行，否则 Writer 使用字面 tree codec。Snapshot adapters 不解释对象或复制检索逻辑。

`CanonicalRelation` 是独立 Relation Address 的值：至少两个带角色的结构化 ObjectRef 端点，不能重复相同角色与引用。端点可以跨仓，Writer 只校验信封并写入关系所在仓，不访问端点 authority。引用不授予读权，也不改变 Schema 同仓解析。关系存储坐标与端点引用坐标分别保留；一跳候选由③查询 Dataset 内的关系存储仓，再按同一 basis 回读。

`Hydrator` 是可替换的固定版本正文读取端口，公开 `ReadMany` 与 `ReadAddress`；它保留 Repository、
commit、完整对象或 Address 的读取形状，且只交付 Snapshot 声明。上层可注入缓存，② Reader
不持有其实现或生命周期。`ValidateHydratedObject` / `ValidateHydratedAddress` 统一校验返回值的
ObjectRef、版本、对象及 Units/Declarations 所属；动态 State hydrate 与当前权限交付在此之后。

`kr://kc/system` 是应用根按部署声明只读打开的实际 Snapshot 仓，发布完整的产品自描述对象：基础 Schema、模型、操作入口、运行机制、能力说明、使用指南和导航关系。
跟踪源是 `system/`（`go:embed`），按 Canonical 树组织；源 `readme.md` 发布为根
`README.md` 导读，`_schemas/` 放声明，`concepts/` 解释模型与依据，`interfaces/` 解释 Writer/CLI 入口，
`mechanisms/` 解释版本底座、认证授权、动态访问、工作分支与条件合并、出站扩展、投影与审计；`capabilities/` 说明用途，`guides/` 说明步骤。内容一对象一目录，
`relations/` 放独立关系。每个文件对应一个 Address，身份来自 frontmatter。
Markdown 切面的值是 `{body}`；定义、用法、入口与使用指南使用仓内 `schema/system/*`
自描述 Schema；模型、入口与机制正文共用 `schema/system/explanation/v1`。这些阅读分类不增加 Dataset 服务 的领域类型。正文产品术语使用对象；Entity / entity 等现行 wire 名称保持。

调用入口的 `commands` 包含 `command`、`help`、`description` 和可选 `action`，从
CLI 登记与 help 同源生成：`go generate ./cli`。普通测试中的
`TestSystemCLIEntrypoints` 检查漂移且不写文件。入口不复制完整参数表，当前调用形状
由叶子 help 提供；实际启用、就绪与授权向当前服务查询。
实际仓通过既有 `snapshot.TreeReader` / `DirectoryReader` 提供文件字节和有界目录
分页，不提供字面写面；精确对象读取不依赖投影。
README 是 Markdown 对象单元，不是 Dataset 服务 库存的 title/summary。协议 Schema 是
`schema/core/readme/v1`（Aspect `readme`，字段 `body` 为字符串）。其 `body` 字段的 `access: [text]` 声明查询能力。人写
`README.md`：frontmatter 用 `entity` / `aspect` 承载 Address，正文是 markdown。
身份仍是 Address；`README.md` 只是人写与 git 直推解释用的有界 `path_hint`。
无 frontmatter 的根 README 不是对象。接入方须在同一仓
发布与 System 出版物 digest 一致的 Schema 副本（`schema_ref` 不跨仓）。
领域分类、owner、质量门槛和投影热状态不进入该对象。`catalog/` DTO 与 `kc show`
的 `repositories` 只列 `{id, schemaCount?}`；读 README 使用对象 URN 的 `/readme` locator，
SEARCH 只按固定版本 Schema 的 `body` access 编译 `text`；旧 pin 保留自己的 Schema 声明。
`ParseSchemaDefinition` 与 `ValidateSchemaInstance` 是 Writer/Reader 共用的协议解释。
Schema 文档可有可选 `description`，说明该实体是什么；不是固定元信息，不是仓 README，
也不是 Dataset 服务 title/summary。`schemas:list` 可带上这一句。
Meta Schema 不向实例继承字段；Entity / Aspect / Relation / Member 是 Address 种类，
不是 System 仓里要「也列出来」的四种实体。
System Repository 的完整出版物（值、Address、Schema 绑定与路径）须与二进制一致；
实际 Snapshot commit 同时固定解释单元和文件字节，所有导读、入口和关系都参与版本依据。部署必须配置系统仓 binding，
`kc deployment init --config deployment.yaml` 把整份出版物经 Writer 写入绑定的空 LakeFS/Gitea Snapshot；
`kc deployment system publish --config deployment.yaml` 可显式重试发布或核对。启动只核对并读取实际仓，不补发布、不挂载内置仓；
已占用仓核对整份出版物，缺失、不一致或包含额外内容文件时拒绝，不能用新说明覆盖旧版本。Domain Schema 文档的
JSON Schema 词表在 `schema-document.schema.yaml`，只用于对账，不替代 Go 校验器。
时间标量包括 `date`（日历日期）以及 `datetime` / `timestamp`（带时区的 RFC3339 时间）；
实例验证保留 Canonical 的原始字符串、时区与精度，检索层再按逻辑类型正规化。
时间精度为纳秒；小数第九位以后只能是等价的补零，不能静默截断有效精度。
`text/filter/sort` 用于已定义的标量字段，包括 `object_ref_list` 的多值字符串；
`object/record/array/relation_endpoint_list` 保留为可读取的复合对象值，不能直接声明
未定义的标量访问。`keyed_collection` 中各 Member 的标量字段仍可声明访问。
`BreakingSchemaChanges` 约束同一 Domain Schema object ID
的值结构只能做兼容演进；字段 access 可随版本增加、收窄或移除，其逻辑摘要变化由投影维护处理。
单仓 Schema 发现由应用层 `schemas:list` 点名已发布实体，不是 Schema 正文或合同摘要。

Address/pattern、必填与 `additionalProperties` 对每个 `schema/*` 无条件生效；省略
`metaSchema` 只是省略文档声明，不是跳过校验的开关。可选 `SchemaReferrerLocator` 是有界反向
`schema_ref` 索引，供 Schema 发布证明既有实例仍然合法；它必须由版本化索引在同一 basis 回答，
不得退化为 Snapshot 扫描。

`object/writer` 接收 Object `ChangeSet`；⓪ `snapshot.Store` 不接收 PUT/REMOVE。Tree provider 在唯一的②→⓪接缝上编译为 `TreeChangeSet`；两条路径共享根包 `units.go` 的唯一 apply/assemble 语义，并由差分 conformance 约束。

批量草稿可直接写成 `*.yaml`、`*.aspect.yaml` 或 `README.md`：frontmatter 声明
Address（`object_id`/`aspect_name`，Markdown 也接受 `entity`/`aspect`）与
`schema_ref`。YAML/JSON 正文是结构化值；README 正文是 `body` 字符串。一个文件
对应一个 Address；`kc writer commit --dir` 对照当前版本求差后进入 Writer。`kc diff --dir` 用同一对照，不写仓。已接入的 tree 仓也可以对 published
ref 直推：Reader 按文件 frontmatter 解释，live 投影对 HEAD 对账。格式转换不得
夹带 source key 映射或领域建模逻辑。HTTP Writer 仍收 ChangeSet。
`schema/*` 默认写入仓内唯一的 `_schemas/` 目录；实例按 `schema_ref` 对应的实体类型
分目录（`metrics/`、`tables/`），不再使用 `objects/` 前缀。身份仍是
`object_id`，目录只是 `path_hint`。

Binding 声明在 Domain Schema Canonical `origin` 上；运行状态、cursor、watermark 和凭证留在墙外 Materialization Runtime。`kc access` 用 origin + 实体 `object_id` 取回该 Aspect，不另存空实例。ResourceDescriptor `origin` 给 `kc invoke`。消费侧 `liveaccess/service` 可通过注入的 State 端口返回观察值及 observation basis，但不会把它写回 Repository 或伪装成 commit 中的值。

对象定义直接描述含义、边界与组成，不增加通用资源包装。插件实现声明组织为
`interfaces` 集合的 Member，每个实现由 Schema 约束并引用插件合同；合同、实现与调用任务是
LiveAccess 插件的内部机制（`liveaccess/handler`、`liveaccess/dispatch`），对象层不认识它们。
`liveaccess/contract.LiveAccessInterfaceV1` 是唯一已安装的合同；ResourceDescriptor 仍是 `kc access` 的声明形式。
声明可读、Schema 合法、处理器存在、调用获准及运行可用分别证明。

system 的 `object-interfaces/` 出版机器合同，`interfaces/` 仍解释 Writer/CLI 操作入口；
Schema 的字段 `access` 出版逻辑查询声明。扩展合同在普通仓发布，处理器独立装配，不追加只读系统仓。

目录内聚合整个②垂直面：根包拥有协议类型与只读 Repository 视图，`reader/` 负责固定 commit 上的对象解释，`writer/` 负责 Object ChangeSet 的 COMMIT（可写指定分支）与条件合并 MERGE。字面路径写入不在本目录，见 `snapshot/`；检索合同不在本目录，见 `discovery/`。

`semanticview/` 只把已经组装的固定版本 `ObjectValue` 渲染成带 `_kc` 坐标的消费 YAML；
它不枚举 Repository、不写回，也不拥有投影生命周期。

`service/` 拥有固定读写任务；`cache/` 实现可选 Hydrator；`maintenance/` 拥有有界遍历、变更解释和独立派生协调。Reader 不扫描、不维护派生、不执行来源访问。Interfaces、Discovery、LiveAccess 与完整 SystemRepo 出版均为外部能力，基础发布与读取不依赖它们。

`cli/` 拥有对象命令、参数、帮助、结果视图与对象专属的既有命令扩展，通过公开贡献由 KC
宿主装配；`client/` 拥有独立 Object HTTP SDK，只消费公开类型和注入的认证传输。
中央 CLI／客户端只装配本模块入口，不复制对象规则。基本 READ／PUT／REMOVE 只用对象 URN＋locator；目录草稿 diff／commit 与回执查询仍由应用流程组织。
