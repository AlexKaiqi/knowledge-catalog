# object/reader/

**Reader 是声明读取面**：在已经钉死的 Snapshot commit 上做 **② 对象解释**。它不写仓、不调用动态 runtime。拼装 Aspect、`object_id`、来源信封与通用 Schema 解释从这里开始；Binding 等能力声明可固定读取，origin、runtime、protocol、操作和 Descriptor 的专属解释由 `liveaccess` 唯一提供。Dataset 服务 `ResolveDataset`（①）只给出 `{repo → commit}`；`reader.Open` 才骑在这次坐标上。面向消费者的逻辑 READ 再由 `liveaccess` 组合本 Reader 与注入的 State runtime；`AccessSpec`、查询和结果合同属于 `discovery/`。

树 Reader 读取选中 Member 后，以其固定 SchemaRef 对应的 binding namespace metadata
区分静态与绑定来源。派生声明可只带 `ValueSource.Kind=binding`，不猜测 State/Stream、
runtime 或操作；这是待解析的读取分类，不是可写入或可执行的 inline Binding。
LiveAccess 在原固定范围读取 Schema 后才取得实际声明；分类不读取私有 Schema 正文。
Native Provider 沿既有声明合同诚实给出来源类别，默认 Snapshot 仍可独立读取。

对外入口是 **Reader**（D26）。它也是应用装配处的 Object Service：Dataset 服务 交付
`snapshot.Store` 后，`Reader.Lookup` 才包装为结构感知的 Repository。Access 仍是领域名：读取协议 + Provider，不必是远程服务。

`Require`、`Wrap` 和 Dataset`Member` 返回的实际值只持有对象读取能力，不能取得
CreateRef、Merge 或 Archive；原生 Provider 藏在私有读接口字段中，不能通过导出字段取得原写口。原生对象 Provider 也通过本包的只读视图交付：已选定的
有界读取端口转发给 Provider，缺少端口明确返回缺能力，不降级为文件树解释或扫描。

两种读目标不是两套协议。坐标来自 `snapshot/dataset.Service.ResolveDataset`；拼装在本包：

```text
单 Repository Commit  Reader.resolve / read / log / diff  维护方：精确版本上的对象任务
Workspace               ResolveDataset → reader.Open → Serving   消费方：成员 union，不覆盖；调用方不传仓/commit
```

消费 CLI 只用对象 URN＋locator；可用 `--dataset` 限定交付范围。对象身份始终带来源 Repo，
不会从 Dataset 中的同名对象猜来源。精确历史读取保留对象引用和返回的 commit。

符号名只解析一次。`--ref refs/heads/main` 或 Workspace selector 在请求开始变成 `commit_id`；命令内不得跟随 `latest`。Agent 消费应 `--dataset`。跨命令跟已发布分支。

文件型 Member 的已选声明以 Writer 维护的固定 Binding namespace locator 判定
`ValueSource` 的 Snapshot/Binding 类别；显式采用旧 Schema commit 时判定仍用该 commit。
这个判定只读取固定定位元数据，不读取 Schema 正文或外部来源，也不解释 origin、runtime、
protocol 或操作。Binding 类别不能被固定 Member 的占位值掩盖；真实 Schema 的读取和取值
仍经 LiveAccess 的同一作用域。普通 Snapshot Member 不要求其 Schema 正文进入 Dataset。
原生声明的类别由 Provider 负责：nil 沿已有合同表示默认 Snapshot，动态声明必须明确
提供既有 Binding 类别。本包保留其声明，不调用可能读取正文的原生 BindingLocator 来猜测。

## 谁被创建

Reader 不创建仓对象。它产出的是读结果和可丢的访问状态：

| 产物 | 怎么来 | 之后 |
|---|---|---|
| Resolution / ObjectValue | `Resolve` / `Read` / `ReadAddress` / `List` | 声明/快照值钉在这次的 commit 上；不是动态 observation |
| ObjectRevision / ObjectDiff | `Log` / `Diff` | 对象历史三问之一，不是 git log |
| ProvenanceTrace | `GetProvenance` | **本对象各单元信封**；不爬 `sourceRefs` |
| GroundingCitation | `NewGroundingCitation(READ 结果)` | 给 Application/UI，不是仓对象 |
| Workspace checkout | `WriteCheckout`（内部机制，当前无公开 CLI） | 可丢 grep 树；钉这次 DatasetPin；不是权威 |

联邦读的 `FederatedValue` 由本包产出，字段与 `ObjectValue` 对齐（另保留 `objectId`）。不要把 public 对象拷进 personal；用户看见的是 Workspace（这次解开的各仓 commit）。逻辑访问计划用 `kc operations access-spec describe`，各仓物理投影用 `kc operations projection describe`；两者都经 Server，不是新对象。

## 三个易混术语

白皮书把「结果字段投影」和「可重建索引」都叫 Projection。协议拆开（设计 7.3）：

| 名称 | 是什么 | 不是什么 |
|---|---|---|
| `AspectSelector` | 显式 READ 时对拼装对象的 aspect `include` / `exclude` | 不是 SEARCH 结果形状，不是索引声明。类型在 `object/` |
| `retrieval.EvaluationProjection` | Refine 时 judge/scorer **可见字段**白名单 | 不是检索文档，不是索引 |
| Access Projection | 从 pinned commit **可重建**的 lexical 索引 | 不是 Canonical，不是 ObjectRef 来源 |

`READ(ref)` 默认拼 `{ aspectName: value }`；调用方用 `AspectSelector` 裁。`readAddress` 读单维护单元。Entity blob（无 `units`）不受 selector 影响。

`permissions` 是 SOURCE 对象，与其他 Aspect 同构。GRANT 正文通常不声明 `text`；若需要按明确字段发现则声明 `filter`。SEARCH 命中后仍回读完整对象，不能靠 selector 或 stored payload 塑造结果。

## 文件（按回答的问题拆）

| 文件 | 负责 |
|---|---|
| `reader.go` | `Reader`：构造、精确读 `RESOLVE` / `READ`（Ref 和 Address）、`LIST` |
| `repository_service.go` |Dataset 服务/Snapshot → Object 包装；`ReadMany` 只在单次调用内共享同 commit tree/解析结果 |
| `native_repository.go` | 原生对象 Provider 的只读视图；转发固定读取与有界定位端口，不暴露 Provider 的写能力 |
| `reference_reader.go` / `reference_units.go` | 同一固定元数据计划选择所需维护单元；分别交付最终选值和未裁剪的完整单元 |
| `metadata_adoption.go` | 从固定派生坐标交付完整 Schema 引用；不读取对象或兄弟正文 |
| `schema.go` | `DESCRIBE_SCHEMA` 编排：固定 commit 上解析 `schema/*` / `schema_ref` |
| `history.go` | 三问：`LOG` / `DIFF` / `GET_PROVENANCE`（设计 7.5；不可互换） |
| `citation.go` | `GroundingCitation`：READ 结果的消费端投影（D12） |
| `checkout.go` | Workspace 只读检出：`仓/object_id.json` + `.kc-pin.json`（grep Provider） |

Relation 查询不在 Reader 枚举或扫描。合同与候选分页在 `discovery/`，执行在 `index.RelationsAt`；它先要求 exact-basis 投影 READY，再只对当前 CandidateRef 页执行同 commit `ReadMany` 并复核 Canonical。无 provider 必须显式 `CAPABILITY_UNSATISFIED`，不得降级为 authority scan。`DESCRIBE_SCHEMA` 复用 Writer 的 Meta Schema 解释，只接受 `schema/*` 上的 `text / filter / sort` 与逻辑类型；`key / summary / stored` 和物理引擎词会失败关闭，并返回规范化 `metaSchema` 与 `additionalProperties`。

tree-backed authority 的 Writer 在同一 commit 写入每对象的维护坐标／路径和 Schema、Binding
命名空间目录。它们只保存身份及单元坐标，用于有界读取，不含 Relation endpoint/type/role、
正文或过滤字段，因此不是检索投影。旧 `.kc/object-units.index` 保持固定读取兼容，
目录升级通过显式维护完成，消费请求不读正文推导缺失目录。业务直推 published
ref 时，Reader 仍按文件 frontmatter 解释（仓根 `README.md` 是有界约定路径）；增量投影用
`ChangedPaths` 上的单元路径 hydrate，不要求该 commit 先有 Writer locator。两种 authority 由同一 Reader/Writer conformance 验收。
合法 README 的精确 Address 与内部字段引用也沿这一个固定路径读取；先检查发表路径，
再核对该 commit 的实际 Schema 和 Address。此入口不补推普通单元坐标，也不吞掉定位元数据错误。

索引在 **Repository 之上**，实现在独立包 `discovery/projection/`（不进 Writer / Dataset 服务 核心）。逻辑查询与结果合同在 `discovery/`；OpenSearch provider 逐 clause Probe 再返回 CandidateRef，命中后回读这次解开的 Canonical。未配置 provider 时只保留精确读取能力。完整边界见 `discovery/README.md` 与 `discovery/projection/README.md`。

Reader 包装后的 Repository 实现
`object.BatchReadStore`，默认 tree locator 在一次批量调用内只读一份固定 commit 的 manifest，
再读取各唯一对象对应的 unit bytes；重复或空身份不产生额外读取。这次调用结束后不保留
`object_id → ObjectValue`。Adapter 只可缓存原始 tree/blob/transport。可选固定正文缓存由独立 `object/cache` 实现，宿主经 Hydrator 注入；缓存不包含当前授权或动态观察。

## 精确读

Interface 合同与实现解析由独立 `interfaces` 消费本 Reader 的固定读口；Reader 不解析或分派处理器。辅助 Schema 与跨仓引用仍受本次固定范围约束，不因能力声明扩大读取范围。

```text
READ(ref, commit, selector?)   → 拼装后按 AspectSelector 裁
READ(address, commit)          → 单单元 Canonical（digest 是该单元）
```

拼装是读策略，不是存储形状；调用方不必知道 authority 的物理路径或表结构。

`ReadReference(object.Reference, commit)` 共用 Object 的 URN 与 JSON Pointer 合同。
tree-backed 读取先按同版本派生的 Address／path 元数据选择单元，再读取必要正文；Member
内部字段可整读该 Member，其他切面和成员不读取。旧的 paths-only 元数据仍支持完整读取，
精确选择须先通过显式维护重建坐标，不能靠请求时读完正文补推。原生 Provider 的非根选择
须提供明确规划端口，缺能力即失败。Dataset 的引用指定来源 Repo，选择涉及的所有路径
必须在本次发表范围内，部分集合不能裁成完整集合。

`ReadReferenceUnits` 共用同一元数据计划和 Dataset 路径边界，交付未做 leaf 裁剪的
根 `object.Unit`，每个 `Digest` 是完整维护值摘要。固定声明还没有动态 leaf 时，
LiveAccess 可先采用所选单元再选值。原生 Provider 必须显式提供此端口；返回身份漂移、
未选坐标、重复单位或不匹配摘要均失败，不以完整对象读取代替所缺能力。

`ReadAddress`、`ResolveAddress` 与其路径限定入口先按同版本维护坐标选择一个单元，
只读该单元正文；传入多个允许路径也不读取未选兄弟。
`SchemaAdoptionLocator.AdoptedSchemaRefs` 只从固定元数据交付实际采用的完整引用，
保留 `SchemaRef@commit`，已知无 Schema 与旧元数据缺证明分开。Dataset 先证明完整对象
在发表范围内；旧 metadata 缺能力时明确失败，不扫描或读兄弟补推。
引用的 Entity／origin／操作含义仍由消费能力解释，Reader 不提供新业务 READ 原语。

完整对象读取的 `Digest` 对应完整组装内容；单元和深字段返回所属完整维护单元的摘要与
Address。Members 集合没有一个独立维护单元时不返回可写摘要。`Reference`／`Locator`
保留实际选择，`Declarations` 只包含所读单元；来源、Schema 和固定 commit 继续沿既有合同。

`Reader.SetHydrator` / `Serving.SetHydrator` 接受宿主装配的 `object.Hydrator`。
端口只在固定 commit 的完整对象或 Address READ 上使用；nil 保留直接权威读取。
Reader 不拥有缓存实现或生命周期，`Require` 返回的维护/声明能力仍直接解释 Repository。
SEARCH 与 RELATIONS 的消费回读由 `index.Index.SetHydrator` 使用同一实例；共享缓存实现见
[`object/cache`](../../object/cache/README.md)。调用方不得以缓存绕过当前授权或动态 State lookup。

## 历史三问

不要合成一个 ORIGIN：

```text
LOG              这个对象在 Snapshot 历史上何时变成各 digest？   → 有界 ObjectLog 页（logs + continuation）
DIFF             两个 pinned commit 上这个对象的值差是什么？     → ObjectDiff（Reader API；无公开 CLI）
GET_PROVENANCE   这个对象在该 commit 上各单元贴了什么信封？     → ProvenanceTrace.chain
```

`GET_PROVENANCE` 不做 PROV 推理。Application 若要沿 `sourceRefs` / `evidence_refs` 再读，必须另发 `RESOLVE` / `READ` / `GET_PROVENANCE`。

`kc catalog audit` 是登记表 git 历史（`Dataset 服务.Log`），不是成员 `LOG`。Dataset 服务 当前态是 `kc show`。

生产 SEARCH、RELATIONS、continuation 与 Refine 合同见 [`discovery/README.md`](../../discovery/README.md)。

## CLI

```bash
go run ./cmd/kc -- read --repo kr://acme/public/core --object ETLTask:job-1 --commit <id>
go run ./cmd/kc -- read --repo kr://acme/public/core --object ETLTask:job-1 --aspect io --commit <id>
go run ./cmd/kc -- provenance --repo kr://acme/public/core --object ETLTask:job-1 --ref refs/heads/main
go run ./cmd/kc -- relations --repo kr://acme/public/core --object Table:orders --relation-type contains --role member --ref refs/heads/main
go run ./cmd/kc -- log --repo kr://acme/public/core --object ETLTask:job-1 --ref refs/heads/main
go run ./cmd/kc -- resolve --repo kr://acme/public/core --object ETLTask:job-1 --ref refs/heads/main
# DIFF 目前只有内部 Reader 语义；公开 Client 尚无 typed route。
go run ./cmd/kc -- binding show --repo kr://acme/public/core --object Service:orders --aspect health --ref refs/heads/main
go run ./cmd/kc -- schema describe --repo kr://acme/public/core --ref refs/heads/main
go run ./cmd/kc -- schema describe --repo kr://acme/public/core --object Table:tl.db.t --ref refs/heads/main
```

多源 `read|search --dataset` 与 `operations access-spec describe --dataset` 走同一已发布 basis。

`Serving` 克隆并固定输入 pin，完整对象的每一条 unit path 都必须在同 commit 的清单内；空清单、未知 item 类型和不匹配 commit 不放行。`Member` 返回受相同范围限制的仓视图，Schema、绑定与 Descriptor 不得通过辅助读取绕过切片。Binding namespace 仅交付允许的 Schema 身份与 Reader 自有静态目录的交集，不解析访问声明；非空切片缺安全元数据口时明确缺能力，不信任任意原生分类器去读取范围外正文。专属解释通过 LiveAccess 消费该只读范围。部分对象不以裁剪后的正文冒充 Canonical；已发布的独立 Address 则通过 `object.AddressPathsReader` 在允许的路径内精确读取，不借用整对象缓存。切片不授权仓历史枚举。

仓级入口使用 `--repo`。宿主文件体验用 `kcfs --dataset --root` 经 Workspace File Gateway
在挂载时内部冻结；没有公开 checkout 或 `refine` 命令。

协议与 Aspect 读策略见 [架构总览](../../doc/architecture.md)、[声明式索引](../../doc/design/discovery.md)。


`DescribeRepoSchema` 通过固定 commit 的有界 Schema locator 解释逻辑类型与字段 access。
`SchemaDescription.Definition` 返回完整规范化 `object.SchemaDefinition`，包含必填、引用类型、
额外字段规则与来源等约束；`Digest` 覆盖这一完整定义。既有 `Fields` 保持字段类型与 access
摘要视图，不能用该投影判断两份 Schema 的约束相同。
查询访问合同只从 Schema 产生，普通对象的名字或正文不会覆盖字段声明；Dataset 的 Schema
辅助读取保持相同版本与文件范围，不通过扫描普通对象补齐解释依赖。

`PlanReference` 仅从同版本维护坐标制定授权计划，不取单元正文。对原生 Provider 的结果
复核对象身份、Address 合法性、选取交集、重复与表示冲突；拒绝凭未选或其它对象的坐标
取得 Aspect 授权。整对象中的同名 JSON 字段不构成 Aspect。Dataset 包装只保留固定来源
坐标，真正取值仍逐所需路径核对已接受的发表范围。
