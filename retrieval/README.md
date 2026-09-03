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

每次实际执行 `SEARCH` 或一跳 `RELATION` 后，应用层在 access evidence 之后追加通用
`retrieval.jsonl`：保存逻辑请求、固定 SearchView、最终候选 Ref/rank、provider lane、回读 basis、
候选/ hydrate/丢弃计数、完整性与错误，不复制完整知识正文。成功结果返回 `retrievalEvidenceId`；
feedback 可直接监督该候选窗，或经下游 refine 自动关联。详见
[`docs/OBSERVABILITY.md`](../docs/OBSERVABILITY.md)。

Refine 是可选、Ref-preserving 的 `SEMANTIC_FILTER` / `SEMANTIC_RERANK`。公开
`POST /knowledge/v1/rerank` 接受固定 Workspace 上的显式 KnowledgeRef 集合；应用层逐 Ref 授权并
在同一 pin Canonical 回读、执行 `EvaluationProjection` 后，才调用注入的批量 `Reranker`。Provider
必须把每个输入 Ref 恰好放入一个 RankGroup 或 `unjudged`；未知、重复、遗漏 Ref 均失败关闭。
`topK` 只是输出选择，已评判但未入选者进入 `notSelected`，不能伪装成 `unjudged`。结果记录
provider/model/spec revision/candidate digest/SearchView；未配置 Provider 时仅 rerank 明确返回
`CAPABILITY_UNSATISFIED`，普通 SEARCH 不受影响。`KeywordJudge` / `KeywordScorer` 仍只是确定性
合同夹具，不代表生产模型。

MVP 另提供 `POST /knowledge/v1/search:rerank`：服务端先执行一个无 continuation、最多 200 条的
固定候选窗口，再把同一 `SearchView` 上已授权、已 hydrate 的命中单次 listwise 交给 Reranker。
结果同时返回 retrieval 与 rerank 两阶段证据；原始 provider/lane/local rank 进入候选摘要证据，但
不会发送给语义模型。投影后请求信封限制为每候选 32 KiB、整窗 128 KiB，超限在出站前
`USAGE_INVALID`，不会自动分批改变全局排序语义。

`retrieval/llmhttp` 是 Responses-compatible 的生产适配器：固定 strict structured output、
`reasoning.effort=none`、一次请求、无内部重试。`kc serve` 只有显式指定 `--rerank-model` 或
`KC_RERANK_MODEL` 才启用；endpoint/key 只从进程环境 `OPENAI_BASE_URL` / `OPENAI_API_KEY` 读取。
每次形成投影候选窗后，应用层把精确模型输入、完整 pre-topK 输出或失败写入独立
`.kc/refine.jsonl`，并在成功响应返回 `refineEvidenceId`。Agent 答案/引用与用户反馈通过该 ID
关联；可训练样本从 refine + feedback 派生，模型输出本身不被当成标签。详见
[`docs/OBSERVABILITY.md`](../docs/OBSERVABILITY.md)。

```bash
set -a; source "$HOME/.env"; set +a
go run ./cmd/kc -- serve --home /tmp/kc-demo --auth local --rerank-model gpt-5.6-luna

# 付费真实模型验收（普通测试默认跳过）
KC_LIVE_LLM_RERANK=1 go test ./retrieval/llmhttp -run '^TestLiveLunaListwiseRerank$' -count=1 -v
```

```bash
go run ./cmd/kc -- knowledge search --repo kr://acme/public/core --query runbook
go run ./cmd/kc -- knowledge search --repo kr://acme/public/core --eq db=tl --query events
go run ./cmd/kc -- knowledge search --repo kr://acme/public/core --contains name=order
go run ./cmd/kc -- operations projection describe --repo kr://acme/public/core
go run ./cmd/kc -- operations projection sync --repo kr://acme/public/core --ref refs/heads/main
```
