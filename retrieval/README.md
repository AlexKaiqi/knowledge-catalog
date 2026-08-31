# retrieval/

③ Retrieval：本目录根拥有 `AccessSpec`、`SearchRequest`、`SearchResult`、continuation 与 Refine 等逻辑合同；`index/` 拥有 Candidate/Projection 端口和执行编排；子目录把端口翻译到具体介质。

| 目录 | 定位 |
|---|---|
| 根包 | 逻辑字段、查询、结果、continuation、Refine；不持物理索引状态 |
| `opensearch/` | OpenSearch 公开装配入口；类型化、代际化的规模投影 |

Provider 只返回带 basis 的 `CandidateRef`，不得把 `_source`、stored field 或物理 score payload 当 Canonical 返回。

Workspace 是调用范围，不是检索字段。物理文档不得保存 Workspace/PinID；上层从
ResolvedWorkspace 为每个已授权 `(repository, commit)` 生成 fragment，复用对应投影后再合并。
OpenSearch 多 index、`_msearch` 或 PinID 级短期 alias 只是可丢优化。

```text
SearchRequest → RetrievalPlan → CandidateRef
              → knowledge/reader READ（同一 basis）
			  → workspace: knowledge/serving State hydrate
              → SearchResult(SearchView, Completeness, KnowledgeHit)
```

`SearchRequest` 有两种互斥形态：既有 `clauses` 继续表示隐式 `All`；组合查询使用
`expression = Clause | All | Any`，`SORT` 独立留在请求级，不能进入表达式。两种形态不能混用。
表达式深度和叶子数有协议上限，防止嵌套输入把 planner/provider 变成无界工作。`Not` 暂不进入
逻辑合同：只有物理计划证明存在有界全集时，补集才是可执行的候选定位。现有 CLI flags 仍只生成
兼容的隐式 `All`；不为组合查询发明字符串 DSL。typed `POST /knowledge/v1/search` 使用
`expression` 传树、使用 `order` 传请求级 `SORT`，并拒绝与旧 `query/match/equal/...` 或旧
`sort` 混用。

Provider 仍逐叶子 `Probe`；显式表达式还必须实现 `index.ExpressionProber`，独立证明自己兑现
`All/Any` 组合。只有叶子能力、没有组合证明的 Provider 在执行前返回
`CAPABILITY_UNSATISFIED`，不得把叶子均支持推导成任意组合均支持。

命中后的 Snapshot 回读必须使用请求开始时钉死的 commit，不能改读 HEAD。纯 Snapshot 查询使用 Snapshot projection；涉及 State Binding 字段时使用独立动态 projection，并从其同 revision Serving State 回读。`SearchView` 只携带紧凑 projection revision，每个命中的 observation basis 放在 `KnowledgeVersion.observations`，避免结果信封随全库规模增长。

Refine 是可选、Ref-preserving 的 `SEMANTIC_FILTER` / `SEMANTIC_RERANK`：输出只能来自输入 Ref；`UNKNOWN` 与未评判必须区分；参考 `KeywordJudge` / `KeywordScorer` 不代表生产模型。

```bash
go run ./cmd/kc -- knowledge search --repo kr://acme/public/core --query runbook
go run ./cmd/kc -- knowledge search --repo kr://acme/public/core --eq db=tl --query events
go run ./cmd/kc -- operations projection describe --repo kr://acme/public/core
go run ./cmd/kc -- operations projection sync --repo kr://acme/public/core --ref refs/heads/main
```
