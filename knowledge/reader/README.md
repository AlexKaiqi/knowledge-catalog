# knowledge/reader/

**Reader 是声明读取面**：在已经钉死的 Snapshot commit 上做 **② 知识解释**。它不写仓、不调用动态 runtime。拼装 Aspect、`object_id`、来源信封与 Binding 解析从这里开始。Catalog `ResolveWorkspace`（①）只给出 `{repo → commit}`；`reader.Open` 才骑在这次坐标上。面向消费者的逻辑 READ 再由 `knowledge/serving` 组合本 Reader 与注入的 State runtime；Schema 的知识解释留在本包，`AccessSpec`、查询和结果合同属于 `retrieval/`。

对外入口是 **Reader**（D26）。它也是应用装配处的 Knowledge Service：Catalog 交付
`snapshot.Store` 后，`Reader.Lookup` 才包装为结构感知的 Repository。Access 仍是领域名：读取协议 + Provider，不必是远程服务。

两种读目标不是两套协议。坐标来自 `catalog.ResolveWorkspace`；拼装在本包：

```text
单 Repository Commit  Reader.resolve / read / log / diff  维护方：精确版本上的对象任务
Workspace               ResolveWorkspace → reader.Open → Serving   消费方：成员 union，不覆盖；调用方不传仓/commit
```

消费 CLI：`kc knowledge read --workspace team-space --object …`（不要 `--repo` / `--ref` / `--commit`）。这次坐标由 `kc catalog workspace resolve --workspace` 生成 `ResolvedWorkspace` pin。维护方核对仍用 `kc knowledge read --repo --commit|--ref`。

符号名只解析一次。`--ref refs/heads/main` 或 Workspace selector 在请求开始变成 `commit_id`；命令内不得跟随 `latest`。Agent 消费应 `--workspace`。跨命令跟已发布分支。

## 谁被创建

Reader 不创建仓对象。它产出的是读结果和可丢的访问状态：

| 产物 | 怎么来 | 之后 |
|---|---|---|
| Resolution / KnowledgeValue | `Resolve` / `Read` / `ReadAddress` / `List` | 声明/快照值钉在这次的 commit 上；不是动态 observation |
| ObjectRevision / ObjectDiff | `Log` / `Diff` | 对象历史三问之一，不是 git log |
| ProvenanceTrace | `GetProvenance` | **本对象各单元信封**；不爬 `sourceRefs` |
| ResolvedBinding | `ResolveBinding` | 固定声明 commit/digest；只解析 inline 或 ResourceDescriptor，不调用 runtime；交给 `knowledge/serving` |
| GroundingCitation | `NewGroundingCitation(READ 结果)` | 给 Application/UI，不是仓对象 |
| Workspace checkout | `WriteCheckout`（内部机制，当前无公开 CLI） | 可丢 grep 树；钉这次 WorkspacePin；不是权威 |

联邦读的 `FederatedValue` 由本包产出，字段与 `KnowledgeValue` 对齐（另保留 `objectId`）。不要把 public 知识拷进 personal；用户看见的是 Workspace（这次解开的各仓 commit）。逻辑访问计划用 `kc operations access describe`，各仓物理投影用 `kc operations projection describe`；两者都经 Server，不是新对象。

## 三个易混术语

白皮书把「结果字段投影」和「可重建索引」都叫 Projection。协议拆开（设计 7.3）：

| 名称 | 是什么 | 不是什么 |
|---|---|---|
| `AspectSelector` | 显式 READ 时对拼装对象的 aspect `include` / `exclude` | 不是 SEARCH 结果形状，不是索引声明。类型在 `knowledge/` |
| `retrieval.EvaluationProjection` | Refine 时 judge/scorer **可见字段**白名单 | 不是检索文档，不是索引 |
| Access Projection | 从 pinned commit **可重建**的 lexical 索引 | 不是 Canonical，不是 KnowledgeRef 来源 |

`READ(ref)` 默认拼 `{ aspectName: value }`；调用方用 `AspectSelector` 裁。`readAddress` 读单维护单元。Entity blob（无 `units`）不受 selector 影响。

`permissions` 是 SOURCE 知识，与其他 Aspect 同构。GRANT 正文通常不声明 `text`；若需要按明确字段发现则声明 `filter`。SEARCH 命中后仍回读完整对象，不能靠 selector 或 stored payload 塑造结果。

## 文件（按回答的问题拆）

| 文件 | 负责 |
|---|---|
| `reader.go` | `Reader`：构造、精确读 `RESOLVE` / `READ`（Ref 和 Address）、`LIST` |
| `repository_service.go` | Catalog/Snapshot → Knowledge 包装；`ReadMany` 只在单次调用内共享同 commit tree/解析结果 |
| `schema.go` | `DESCRIBE_SCHEMA` 编排：固定 commit 上解析 `schema/*` / `schema_ref` |
| `schema_parse.go` | Binding/兼容形状所需的无 I/O 辅助；Domain Schema 的规范解析复用根包 `ParseSchemaDefinition` |
| `history.go` | 三问：`LOG` / `DIFF` / `GET_PROVENANCE`（设计 7.5；不可互换） |
| `binding.go` | 在固定 commit 解析 Aspect ValueSource / ResourceDescriptor，返回 ResolvedBinding |
| `citation.go` | `GroundingCitation`：READ 结果的消费端投影（D12） |
| `checkout.go` | Workspace 只读检出：`仓/object_id.json` + `.kc-pin.json`（grep Provider） |

Relation 查询不在 Reader 枚举或扫描。合同与候选分页在 `retrieval/`，执行在 `index.RelationsAt`；它先要求 exact-basis 投影 READY，再只对当前 CandidateRef 页执行同 commit `ReadMany` 并复核 Canonical。无 provider 必须显式 `CAPABILITY_UNSATISFIED`，不得降级为 authority scan。`DESCRIBE_SCHEMA` 复用 Writer 的 Meta Schema 解释，只接受 `schema/*` 上的 `text / filter / sort` 与逻辑类型；`key / summary / stored` 和物理引擎词会失败关闭，并返回规范化 `metaSchema` 与 `additionalProperties`。

Gitea 等 tree-backed authority 的 Writer 在同一 commit 写入 `.kc/knowledge-units.index`。它只保存
`object_id → unit path` 以及 Schema/Binding 精确读取所需的 identity 集合，用于有界 `ReadMany`；
不含 Relation endpoint/type/role、正文或过滤字段，因此不是检索投影。Dolt 使用自己的主键表完成
同一精确读取合同。两种 authority 由同一 Reader/Writer conformance 验收。

索引在 **Repository 之上**，实现在独立包 `index/`（不进 Writer / Catalog 核心）。逻辑查询与结果合同在 `retrieval/`；OpenSearch provider 逐 clause Probe 再返回 CandidateRef，命中后回读这次解开的 Canonical。未配置 provider 时只保留精确读取能力。完整边界见 `retrieval/README.md` 与 `index/README.md`。

生产 hydrate 不在底座缓存 Knowledge object。Reader 包装后的 Repository 实现
`knowledge.BatchReadStore`，一次候选页只读取一次固定 Snapshot tree；这次调用结束后不保留
`object_id → KnowledgeValue`。Adapter 只可缓存原始 tree/blob/transport。上层产品如需完整对象缓存，在 KC 之上的 retriever lane 实现。

## 精确读

```text
READ(ref, commit, selector?)   → 拼装后按 AspectSelector 裁
READ(address, commit)          → 单单元 Canonical（digest 是该单元）
```

拼装是读策略，不是存储形状；调用方不必知道 authority 的物理路径或表结构。

## 历史三问

不要合成一个 ORIGIN：

```text
LOG              这个对象在 Snapshot 历史上何时变成各 digest？   → 有界 ObjectLog 页（logs + continuation）
DIFF             两个 pinned commit 上这个对象的值差是什么？     → ObjectDiff（Reader API；无公开 CLI）
GET_PROVENANCE   这个对象在该 commit 上各单元贴了什么信封？     → ProvenanceTrace.chain
```

`GET_PROVENANCE` 不做 PROV 推理。Application 若要沿 `sourceRefs` / `evidence_refs` 再读，必须另发 `RESOLVE` / `READ` / `GET_PROVENANCE`。

`kc catalog audit` 是登记表 git 历史（`Catalog.Log`），不是成员 `LOG`。Catalog 当前态是 `kc catalog show`。

生产 SEARCH、RELATIONS、continuation 与 Refine 合同见 [`retrieval/README.md`](../../retrieval/README.md)。

## CLI

```bash
go run ./cmd/kc -- knowledge read --repo kr://acme/public/core --object ETLTask:job-1 --commit <id>
go run ./cmd/kc -- knowledge read --repo kr://acme/public/core --object ETLTask:job-1 --aspect io --commit <id>
go run ./cmd/kc -- knowledge provenance --repo kr://acme/public/core --object ETLTask:job-1 --ref refs/heads/main
go run ./cmd/kc -- knowledge relations --repo kr://acme/public/core --object Table:orders --relation-type contains --role member --ref refs/heads/main
go run ./cmd/kc -- knowledge log --repo kr://acme/public/core --object ETLTask:job-1 --ref refs/heads/main
go run ./cmd/kc -- knowledge resolve --repo kr://acme/public/core --object ETLTask:job-1 --ref refs/heads/main
# DIFF 目前只有内部 Reader 语义；公开 Client 尚无 typed route。
go run ./cmd/kc -- knowledge binding resolve --repo kr://acme/public/core --object Service:orders --aspect health --ref refs/heads/main
go run ./cmd/kc -- knowledge schema describe --repo kr://acme/public/core --ref refs/heads/main
go run ./cmd/kc -- knowledge schema describe --repo kr://acme/public/core --object Table:tl.db.t --ref refs/heads/main
```

`kc knowledge read --workspace` / `kc operations access describe --workspace` 走 Catalog pin。`kc knowledge search --workspace` 按 AccessPlan 分成员检索，并显式报告联邦 coverage。仓级检索和投影维护分别走 `kc knowledge search --repo`、`kc operations projection describe|sync`。宿主文件体验用 `kcfs` 经 Workspace File Gateway 物化固定 pin；没有公开 checkout 或 `refine` 命令。

全文乱翻用检出上的 `rg`；声明了 AccessHints 的过滤仍走 `kc knowledge search --workspace`。不要把 `.kc/repos` 或 `kc serve` 的 tree 当 Workspace。

协议与 Aspect 读策略见 [`docs/KNOWLEDGE_CATALOG_DESIGN.md`](../../docs/KNOWLEDGE_CATALOG_DESIGN.md) 第 7 章、[`docs/ASPECT_ACCESS.md`](../../docs/ASPECT_ACCESS.md)。
