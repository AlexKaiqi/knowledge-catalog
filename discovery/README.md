# discovery/

③ Retrieval：本目录根拥有 `AccessSpec`、`SearchRequest`、`SearchResult`、continuation 与 Refine 等逻辑合同；`discovery/projection/` 拥有 Candidate/Projection 端口和执行编排。声明式索引插件及物理投影实现归 `declarativeindex/`，可选模型处理适配归本模块。

| 目录 | 定位 |
|---|---|
| 根包 | 逻辑字段、查询、结果、continuation、Refine；不持物理索引状态 |
| `projection/` | 通用 Candidate/Projection 端口与固定依据执行编排 |
| `llmhttp/` | 可选 Responses-compatible 模型处理适配 |

OpenSearch 的具体装配入口在 [`declarativeindex/opensearch`](../declarativeindex/opensearch/README.md)，不属于本目录的通用发现合同。

Provider 只返回带 basis 的 `CandidateRef`，不得把 `_source`、stored field 或物理 score payload 当 Canonical 返回。

`RelationRetrieveRequest.Repository/Basis` 定位已准备的关系候选空间；正式 Dataset 一跳查询使用该修订的单份 corpus，底层提供方合同也可定位固定来源投影。`Query.Endpoint` 是独立的端点引用，候选范围不因端点扩大；完整 Relation 在其真实来源仓的固定 commit 回读。端点角色与完整引用必须关联匹配，不能混配多个端点的字段。

Dataset 修订是消费检索范围，不是检索字段。物理文档不保存 Dataset/PinID；声明式索引
把选入单位编译成一个 corpus，跨来源使用同一套打分。Dataset 消费权限先于检索，命中
按相同修订的来源版本向量回读全部选入方面，正文不再逐来源按仓读权裁剪。

## TRAVERSE：有界邻域闭包

`TraverseRequest/TraversePage`（`discovery/traverse.go`）是设计 §7.5 选定的有界类型化遍历合同：
范围在请求开始固定（Dataset 清单 pin 或单仓 commit），沿 `TraverseStep`（关系类型/角色/方向）
逐跳扩展，`MaxHops` 上限 8，`MaxTraverseEdges` 限制整个续传序列的边预算。

- 结果是**闭包增量**而非路径：`nodes` 按 `(repository, object)` 去重并携带最小跳深（`MinHops`
  只过滤节点集合，不过滤边）；`edges` 是入选关系对象（保留自身 `repository/commit` 坐标与完整
  关系端点，每条边恰好返回一次）；不枚举全部路径、不承诺最短路径。
- 引用到范围外对象的 frontier **止步并列 boundary**（`BoundaryOutsideScope`）；boundary 不透露
  关系体之外的任何信息，也不扩大授权（PERMISSIONS `object.traverse`）。
- 页是闭包的不相交增量，`Continuation` 绑定查询摘要、固定范围视图与范围身份（Dataset 为清单
  摘要），变更后按 `PRECONDITION_FAILED` 拒绝；`Exhausted=true` 表示闭包已完整。
- 每一跳复用同一关系执行 lane（`index.RelationsAtContext`：layer ③ 候选 → 同 commit authority
  回读 → 上下文重核），不引入第二条关系执行路径。

```text
SearchRequest → RetrievalPlan → CandidateRef
              → object/reader READ（同一 basis）
			  → workspace: liveaccess State hydrate
              → SearchResult(SearchView, Completeness, ObjectHit)
```

`SearchRequest` 有两种互斥形态：既有 `clauses` 继续表示隐式 `All`；组合查询使用
`expression = Clause | All | Any`，`SORT` 独立留在请求级，不能进入表达式。两种形态不能混用。
表达式深度和叶子数有协议上限，防止嵌套输入把 planner/provider 变成无界工作。`Not` 暂不进入
逻辑合同：只有物理计划证明存在有界全集时，补集才是可执行的候选定位。现有 CLI flags 仍只生成
兼容的隐式 `All`；不为组合查询发明字符串 DSL。typed `POST /object/v1/search` 使用
`expression` 传树、使用 `order` 传请求级 `SORT`，并拒绝与旧 `query/match/equal/...` 或旧
`sort` 混用。

Provider 仍逐叶子 `Probe`；显式表达式还必须实现 `index.ExpressionProber`，独立证明自己兑现
`All/Any` 组合。只有叶子能力、没有组合证明的 Provider 在执行前返回
`CAPABILITY_UNSATISFIED`，不得把叶子均支持推导成任意组合均支持。

命中后的 Snapshot 回读必须使用请求开始时钉死的 commit，不能改读 HEAD。纯 Snapshot 查询使用 Snapshot projection；涉及 State Binding 字段时使用独立动态 projection，并从其同 revision Serving State 回读。`SearchView` 只携带紧凑 projection revision，每个命中的 observation basis 放在 `ObjectVersion.observations`，避免结果信封随全库规模增长。

Dataset 检索中按外部值回答的条件逐条列在结果的 `externalConditions`：字段与建索引时最旧的观察
时间；命中仍按当前观察回读补判。选入 Dataset 的声明若输入 Schema 不在该范围，状态为
`inapplicable`：范围内不可能有带该字段的对象，只碰到它的条件得到完整零命中（`All` 中任一如此即
整体为空，`Any` 中剔除），不当作未声明或不支持；同一请求里的其它条件照常校验。

每次实际执行 `SEARCH` 或一跳 `RELATION` 后，应用层在 access evidence 之后追加通用
`retrieval.jsonl`：保存逻辑请求、固定 SearchView、最终候选 Ref/rank、provider lane、回读 basis、
候选/ hydrate/丢弃计数、完整性与错误，不复制完整对象正文。成功结果返回 `retrievalEvidenceId`；
feedback 可直接监督该候选窗，或经下游 refine 自动关联。详见
[服务边界](../doc/design/service.md)。

Refine 是可选、Ref-preserving 的 `SEMANTIC_FILTER` / `SEMANTIC_RERANK`。公开
`POST /object/v1/rerank` 接受固定 Workspace 上的显式 ObjectRef 集合；应用层逐 Ref 授权并
在同一 pin Canonical 回读、执行 `EvaluationProjection` 后，才调用注入的批量 `Reranker`。Provider
必须把每个输入 Ref 恰好放入一个 RankGroup 或 `unjudged`；未知、重复、遗漏 Ref 均失败关闭。
`topK` 只是输出选择，已评判但未入选者进入 `notSelected`，不能伪装成 `unjudged`。结果记录
provider/model/spec revision/candidate digest/SearchView；未配置 Provider 时仅 rerank 明确返回
`CAPABILITY_UNSATISFIED`，普通 SEARCH 不受影响。`KeywordJudge` / `KeywordScorer` 仍只是确定性
合同夹具，不代表生产模型。

MVP 另提供 `POST /object/v1/search:rerank`：服务端先执行一个无 continuation、最多 200 条的
固定候选窗口，再把同一 `SearchView` 上已授权、已 hydrate 的命中单次 listwise 交给 Reranker。
结果同时返回 retrieval 与 rerank 两阶段证据；原始 provider/lane/local rank 进入候选摘要证据，但
不会发送给语义模型。投影后请求信封限制为每候选 32 KiB、整窗 128 KiB，超限在出站前
`USAGE_INVALID`，不会自动分批改变全局排序语义。

`discovery/llmhttp` 是 Responses-compatible 的生产适配器：固定 strict structured output、
`reasoning.effort=none`、一次请求、无内部重试。`kc serve` 只有显式指定 `--rerank-model` 或
`KC_RERANK_MODEL` 才启用；endpoint/key 只从进程环境 `OPENAI_BASE_URL` / `OPENAI_API_KEY` 读取。
每次形成投影候选窗后，应用层把精确模型输入、完整 pre-topK 输出或失败写入独立
`.kc/refine.jsonl`，并在成功响应返回 `refineEvidenceId`。Agent 答案/引用与用户反馈通过该 ID
关联；可训练样本从 refine + feedback 派生，模型输出本身不被当成标签。详见
[服务边界](../doc/design/service.md)。

```bash
set -a; source "$HOME/.env"; set +a
go run ./cmd/kc -- serve --home /tmp/kc-demo --auth local --rerank-model gpt-5.6-luna

# 付费真实模型验收（普通测试默认跳过）
KC_LIVE_LLM_RERANK=1 go test ./discovery/llmhttp -run '^TestLiveLunaListwiseRerank$' -count=1 -v
```

```bash
go run ./cmd/kc -- search --dataset sales-analysis --query runbook
go run ./cmd/kc -- search --dataset sales-analysis --revision 1 --eq db=tl --query events
go run ./cmd/kc -- search --dataset sales-analysis --contains name=order
go run ./cmd/kc -- operations projection describe --dataset sales-analysis --revision 1
go run ./cmd/kc -- operations projection sync --dataset sales-analysis
```

## RECALL：请求级召回策略（lexical / semantic / hybrid 闭集）

`SearchRequest.Recall`（`discovery/recall.go`）是设计 §8 选定的请求级召回策略闭集：
`lexical`（现役默认，省略字段等价 lexical）、`semantic`、`hybrid`（只保留名字，融合合同另行
裁决，请求一律 `USAGE_INVALID`）。策略不修改第 5 节代数的算子与 envelope 含义。

- `ValidateSearch` 先验：未知策略与 `hybrid` 拒绝；`semantic` 要求请求携带至少一个 MATCH 文本叶
  （legacy clauses 与 typed expression 同规则），纯过滤请求不能形成向量窗口，也不得静默降级为
  lexical。
- 资格先于召回执行：消费方取得请求向量后，`SemanticDatasetWindow` 在固定 Dataset corpus
  的显式向量成员上取一次有界 k-NN 窗口。声明的模型必须由实例真实登记，全文成员不会
  自动变成向量成员；缺提供方、无向量面、模型或 dimension 不符都失败关闭。底层提供方
  合同保留 `SemanticWindowAtContext` 证明同样的近似、同版本回读和不降级风险。
- 结果一律 Approximate：`retrieval.MarkApproximate` 强制 partial completeness，并披露近似来源
  （k-NN 排名窗口、embedding 模型身份）；投影侧候选带 `semantic-vector` lane 的
  `approximate` guarantee 证据。窗口是单次有界 top-K，不分页不续传。


## 声明式索引

采用 `schema/core/index-declaration/v1` 的 `index` 成员是检索能力的唯一来源；Schema 报告
只给出值结构。维护通过固定版本的有界反向 locator 编译声明并保留成员见证。消费侧
`PlanAccess` 先采用 Dataset 的版本与范围，由提供方传入已维护的合同；
`AccessSpecFromAdoptions` 在同一范围按成员见证精确回读验证，不枚举正文或补读仓级 locator。
普通对象正文中的查询样式数据不产生能力。

每个成员列出 Schema 字段定位符、构建方式及用途，实例能力清单逐成员说明支持情况。
逻辑字段仍定位到 `(Schema, Aspect, path)`；声明成员可独立增加、收窄或移除，
不改变 Schema 值结构的兼容性判断。旧 pin 保留原合同；
`AccessDigest` 覆盖逻辑字段、类型和访问能力，变化触发投影重建，普通内容更新保持增量。
提供方能力、物理就绪、范围/授权和同依据回读分别证明，声明不代替任何一项。
