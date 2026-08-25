# reader/

**Reader 是读取面**：在已经钉死的 Snapshot commit 上做 **② 知识解释**。它不写仓、不调用动态 runtime。拼装 Aspect、`object_id`、来源信封与 Binding 解析从这里开始。Catalog `ResolveWorkspace`（①）只给出 `{repo → commit}`；`reader.Open` 才骑在这次坐标上。Schema 在本包编译为 `AccessSpec`；请求执行与 provider 选择属于 `index/`。

对外入口是 **Reader**（D26）。Access 仍是领域名：读取协议 + Provider，不必是远程服务。

两种读目标不是两套协议。坐标来自 `catalog.ResolveWorkspace`；拼装在本包：

```text
单 Repo Commit     Reader.resolve / read / log / diff     维护方：精确版本上的对象任务
Workspace               ResolveWorkspace → reader.Open → Serving   消费方：成员 union，不覆盖；调用方不传仓/commit
```

消费 CLI：`kc read --workspace team-space --object …`（不要 `--repo` / `--ref` / `--commit`）。这次坐标：`kc resolve --workspace`（不带 `--object`）出 `ResolvedWorkspace` pin。维护方核对仍用 `read --repo --commit|--ref`。

符号名只解析一次。`--ref refs/heads/main` 或 Workspace selector 在请求开始变成 `commit_id`；命令内不得跟随 `latest`。Agent 消费应 `--workspace`。跨命令跟已发布分支。

## 谁被创建

Reader 不创建仓对象。它产出的是读结果和可丢的访问状态：

| 产物 | 怎么来 | 之后 |
|---|---|---|
| Resolution / KnowledgeValue | `Resolve` / `Read` / `ReadAddress` / `List` | 钉在这次的 commit 上 |
| ObjectRevision / ObjectDiff | `Log` / `Diff` | 对象历史三问之一，不是 git log |
| ProvenanceTrace | `GetProvenance` | **本对象各单元信封**；不爬 `sourceRefs` |
| ResolvedBinding | `ResolveBinding` | 固定声明 commit/digest；只解析 inline 或 ResourceDescriptor，不调用 runtime |
| Access Projection | `Projection.Build` | 可丢、可重建；命中后回读 Canonical |
| Refine 结果 | `Refine.Filter` / `Rerank` | Ref-preserving；不产生新对象 |
| GroundingCitation | `NewGroundingCitation(READ 结果)` | 给 Application/UI，不是仓对象 |
| Workspace checkout | `WriteCheckout` / `kc checkout --workspace` | 可丢 grep 树；钉这次 WorkspacePin；不是权威 |

联邦读的 `FederatedValue` 由本包产出，字段与 `KnowledgeValue` 对齐（另保留 `objectId`）。不要把 public 知识拷进 personal；用户看见的是 Workspace（这次解开的各仓 commit）。系统状态拼装是 `kc inspect --workspace`（① pin + AccessPlan + 各仓投影），不是新对象。

## 三个易混术语

白皮书把「结果字段投影」和「可重建索引」都叫 Projection。协议拆开（设计 7.3）：

| 名称 | 是什么 | 不是什么 |
|---|---|---|
| `AspectSelector` | 显式 READ 时对拼装对象的 aspect `include` / `exclude` | 不是 SEARCH 结果形状，不是索引声明。类型在 `repository/` |
| `EvaluationProjection` | Refine 时 judge/scorer **可见字段**白名单 | 不是检索文档，不是索引 |
| Access Projection | 从 pinned commit **可重建**的 lexical 索引 | 不是 Canonical，不是 KnowledgeRef 来源 |

`READ(ref)` 默认拼 `{ aspectName: value }`；调用方用 `AspectSelector` 裁。`readAddress` 读单维护单元。Entity blob（无 `units`）不受 selector 影响。

`permissions` 是 SOURCE 知识，与其他 Aspect 同构。GRANT 正文通常不声明 `text`；若需要按明确字段发现则声明 `filter`。SEARCH 命中后仍回读完整对象，不能靠 selector 或 stored payload 塑造结果。

## 文件（按回答的问题拆）

| 文件 | 负责 |
|---|---|
| `reader.go` | `Reader`：构造、精确读 `RESOLVE` / `READ`（Ref 和 Address）、`LIST` |
| `schema.go` | `DESCRIBE_SCHEMA`：`schema/*` / `schema_ref` → Pattern + AccessHints |
| `history.go` | 三问：`LOG` / `DIFF` / `GET_PROVENANCE`（设计 7.5；不可互换） |
| `binding.go` | 在固定 commit 解析 Aspect ValueSource / ResourceDescriptor，返回 ResolvedBinding |
| `search.go` | `Reader.Search`：整包 JSON 包含，**调试**，不当生产检索 |
| `projection.go` | Access Projection（T8）：进程内 contains，定位后回读 Canonical |
| `spec.go` | AccessHints → `AccessSpec`，FieldRef = schema + aspect + path |
| `accessplan.go` | Workspace pin 上每仓一份逻辑 AccessSpec；不是物理索引定义 |
| `retrieval.go` | SearchResult / View / KnowledgeVersion / Completeness / Evidence |
| `searchop.go` | 查询算子 + `AllowsOp`（由属性 `access`+`type` 推出，不是在 schema 上列算子） |
| `refine.go` | 可选 Capability：`SEMANTIC_FILTER` / `SEMANTIC_RERANK`（T10） |
| `citation.go` | `GroundingCitation`：READ 结果的消费端投影（D12） |
| `checkout.go` | Workspace 只读检出：`仓/object_id.json` + `.kc-pin.json`（grep Provider） |

`CAPABILITIES` / `EXPAND_RELATIONS` / `WATCH_UPDATES` 语义已冻结，本包未实现。缺失必须显式（`CAPABILITY_UNSATISFIED`），不能用 grep 冒充向量命中。`DESCRIBE_SCHEMA` 只接受 `schema/*` 上的 `text / filter / sort` 与逻辑类型；`key / summary / stored` 和物理引擎词会失败关闭。

索引在 **Repo 之上**，实现在独立包 `index/`（不进 Writer / Catalog 核心）。`AccessSpec` 是逻辑合同；SQLite/ES provider 逐 clause Probe 再返回 CandidateRef，命中后回读这次解开的 Canonical。公开结果包含 View、KnowledgeVersion、Evidence、Completeness、Claims 与绑定查询视图的 opaque continuation。完整边界见 `index/README.md`。

## 精确读

```text
READ(ref, commit, selector?)   → 拼装后按 AspectSelector 裁
READ(address, commit)          → 单单元 Canonical（digest 是该单元）
```

拼装是读策略，不是存储形状。FileGit 一文件一 Address；调用方不必知道路径。

## 历史三问

不要合成一个 ORIGIN：

```text
LOG              这个对象在 Snapshot 历史上何时变成各 digest？   → ObjectRevision[]
DIFF             两个 pinned commit 上这个对象的值差是什么？     → ObjectDiff
GET_PROVENANCE   这个对象在该 commit 上各单元贴了什么信封？     → ProvenanceTrace.chain
```

`GET_PROVENANCE` 不做 PROV 推理。Application 若要沿 `sourceRefs` / `evidence_refs` 再读，必须另发 `RESOLVE` / `READ` / `GET_PROVENANCE`。

`kc audit` 是登记表 git 历史（`Catalog.Log`），不是成员 `LOG`。Catalog 当前态是 `kc read --catalog`。

## SEARCH

```text
目标路径    SearchRequest 原子 clause（带 mode 的 MATCH、typed filter/range、MISSING/PREFIX、SORT，隐式 AND）
            → RetrievalPlan → CandidateRef → READ 完整 Canonical（同一 basis）
            → SearchResult(View, Completeness, KnowledgeHit[value, version, evidence])
当前路径    index.Index Probe/Retrieve CandidateRef → READ Canonical（同一 commit）→ SearchResult
调试路径    Reader.Search = 整包 JSON 包含；不当生产检索
T8 路径     Projection.Build 进程内 contains（conformance）
可选通道    VECTOR / HYBRID（Capability；当前 Profile 不支持）
```

命中后的 Canonical hydrate 必须使用这次的 pinned commit，不能 silently 改去读 HEAD。SEARCH 始终返回完整知识；裁剪只允许发生在更上层的上下文组装。

## Refine

可选，不是 Base 读。路径：候选 → `SEMANTIC_FILTER` / `SEMANTIC_RERANK` → 再 READ。

- `UNKNOWN`（已看、不可判定）与 `UNJUDGED`（没看完）必须分开。
- 输出 ⊆ 输入 Ref；禁止产生新 Ref、禁止副作用。
- `KeywordJudge` / `KeywordScorer` 是 T10 参考 judge，不是生产模型。
- `EXTRACT_TYPED` 会产生新值，属于 Derivation，不进 Reader。

## CLI

```bash
go run ./cmd/kc -- resolve --repo kr://acme/public/core --object ETLTask:job-1 --ref refs/heads/main
go run ./cmd/kc -- read --repo kr://acme/public/core --object ETLTask:job-1 --commit <id>
go run ./cmd/kc -- read --repo kr://acme/public/core --object ETLTask:job-1 --aspect io --commit <id>
go run ./cmd/kc -- provenance --repo kr://acme/public/core --object ETLTask:job-1 --ref refs/heads/main
go run ./cmd/kc -- list --repo kr://acme/public/core --ref refs/heads/main
go run ./cmd/kc -- log --repo kr://acme/public/core --object ETLTask:job-1 --ref refs/heads/main
go run ./cmd/kc -- diff --repo kr://acme/public/core --object ETLTask:job-1 --from <a> --to <b>
go run ./cmd/kc -- resolve-binding --repo kr://acme/public/core --object Service:orders --aspect health --ref refs/heads/main
go run ./cmd/kc -- describe-schema --repo kr://acme/public/core --ref refs/heads/main
go run ./cmd/kc -- describe-schema --repo kr://acme/public/core --object Table:tl.db.t --ref refs/heads/main
go run ./cmd/kc -- search --repo kr://acme/public/core --query runbook
go run ./cmd/kc -- search --repo kr://acme/public/core --eq db=tl --query events
go run ./cmd/kc -- search --repo kr://acme/public/core --in db=tl,dw --exists db --sort when:asc
go run ./cmd/kc -- describe-index --repo kr://acme/public/core
go run ./cmd/kc -- index-sync --repo kr://acme/public/core --ref refs/heads/main
```

`kc read --workspace` / `kc describe-access --workspace` 走 Catalog pin。`kc search --workspace` 按 AccessPlan 分成员检索，并显式报告联邦 coverage。`kc search --repo` / `describe-index` / `index-sync` 走 `index/`。`kc checkout --workspace` 把这次 `List` 落成 `layout.checkouts/<workspace>/`（路径是身份，不是 git `pathHint`；联邦不合并）。没有 `kc refine`。

全文乱翻用检出上的 `rg`；声明了 AccessHints 的过滤仍走 `search --workspace`。不要把 `.kc/repos` 或 `kc serve` 的 tree 当 Workspace。

协议与 Aspect 读策略见 [`docs/KNOWLEDGE_CATALOG_DESIGN.md`](../docs/KNOWLEDGE_CATALOG_DESIGN.md) 第 7 章、[`docs/ASPECT_ACCESS.md`](../docs/ASPECT_ACCESS.md)。
