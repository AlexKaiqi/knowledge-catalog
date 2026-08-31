# 知识访问可观测性

范围：谁在什么时候访问了哪个固定版本的知识，以及规范 Agent 如何把多次访问和反馈关联成一条可审计 trace。认证提供方不在本协议内。

本文的 trace 是知识访问证据视图，不等于可采样的分布式诊断遥测。系统 metric、日志、distributed trace、健康检查与 SLO 见 [`SYSTEM_OBSERVABILITY.md`](SYSTEM_OBSERVABILITY.md)。

## 身份

每次调用携带一个已经由上游建立的身份上下文：

```json
{
  "principal": "agent:finance-analyst-v3",
  "onBehalfOf": "user:kaiqidong"
}
```

- `principal` 是实际执行调用的主体，必填；CLI 兼容入口仍是 `--as`。
- `onBehalfOf` 是该主体所代理的用户，可选。
- 用户直接访问时，`principal=user:...`，省略 `onBehalfOf`。
- KC 当前只校验字段形状并原样传播，不验证 token、委托签名或 delegation chain。共享服务后续可替换认证器，但不能改变这两个字段的含义。
- 当前授权仍按 `principal` 求值；`onBehalfOf` 是可信上游断言和审计事实，尚不参与授权交集计算。

HTTP 本地模式使用 `X-Kc-As` 与 `X-Kc-On-Behalf-Of`。认证模式由认证器注入 `principal`，`onBehalfOf` 暂按请求上下文透传。生产接入委托认证后，网关必须只接受经过验证的 `onBehalfOf`。

## 访问账

`knowledge.read/search/rerank/relations/provenance/log/schema.describe` 与 `file.read` 等 semantic action 完成后，应用服务追加 `.kc/access.jsonl`。成功、失败和拒绝都记录；`search:rerank` 记录 SEARCH 实际命中的同一批固定 basis 知识访问，候选摘要保留 provider/lane/originalRank，但这些物理排名不发送给模型。成功返回的每条知识记录为：

```json
{
  "evidenceId": "ev_01...",
  "occurredAt": "...",
  "identity": {
    "principal": "agent:finance-analyst-v3",
    "onBehalfOf": "user:kaiqidong"
  },
  "action": "read",
  "requestId": "req-42",
  "workspace": "finance-board",
  "pinId": "...",
  "decision": "ALLOW",
  "result": "RESOLVED",
  "knowledge": [{
    "knowledgeRef": {
      "repository": "kr://acme/org/semantics",
      "commit": "<fixed commit>",
      "object": "Metric:gmv"
    },
    "address": {
      "objectId": "Metric:gmv",
      "aspectName": "definition"
    },
    "observations": [{
      "address": {"objectId": "Metric:gmv", "aspectName": "runtime"},
      "declarationCommit": "<fixed commit>",
      "declarationDigest": "<digest>",
      "basis": {
        "bindingGeneration": "runtime-v7",
        "consistency": "bounded",
        "sourceRevision": "rev-42",
        "observedAt": "2026-08-27T09:00:00Z"
      }
    }]
  }]
}
```

`observations` 只在 Bound State 值实际 hydrate 时出现，使访问证据同时保留声明 basis 与运行时
basis。VFS 只记录固定文件坐标，不产生 observation；失败的 runtime 调用仍记录错误，但不伪造
一次成功观察。

`evidenceId` 由 Recorder 生成，调用方不得提供。只有 event 完整写入并完成 `fsync` 后 Recorder 才把
它作为内部 delivery ack 返回给 facade；覆盖率对账使用 `evidenceId`，不能使用可能重复的
`requestId` 作为唯一键。

访问账是过程证据，不进成员 Repository，不是 provenance，也不改变 Canonical 知识。批量 checkout 除逐对象命中外，还记录 `repository + commit` 快照范围；不会把 mount 路径冒充成文件命中。记录失败会使成功的 facade 响应失败，避免把“已返回但没有访问账”伪装成合规成功。

查询：

```bash
kc operations audit access --trace-id trace-42
kc operations audit access --filter-principal agent:finance-analyst-v3
kc operations audit access --repo kr://acme/org/semantics --object Metric:gmv
```

查询访问账、trace 和 hitmap 复用 `audit` 权限。

## Agent trace 与反馈

规范 Agent 用 trace/span 关联同一任务中的 KC 调用：

```text
traceId trace-42
spanId span-read-1
parentSpanId span-search-1
```

当前 facade 使用 `--trace-id`、`--span-id`、`--parent-span-id` 及对应
`X-Kc-Trace-Id`、`X-Kc-Span-Id`、`X-Kc-Parent-Span-Id`；目标服务入口使用
`traceparent` / `tracestate`。`kc operations audit trace --trace-id trace-42` 按时间合并实际知识访问和反馈：

```bash
kc operations feedback record --workspace finance-board --trace-id trace-42 \
  --as agent:finance-analyst-v3 --on-behalf-of user:kaiqidong \
  --outcome helpful --message "answer accepted"
```

`record-feedback` 只接受已经存在知识访问的 trace id，避免产生无法关联到任何访问的孤立反馈。

这里的“完整 trace”是知识系统边界内的完整调用证据：请求身份、关联 id、授权结果、固定知识版本、访问结果和显式反馈。它不保存模型隐式推理或 chain-of-thought。Agent 绕过应用服务直接读成员 Git、checkout 文件或索引介质时，KC 无法观察逐条访问；`kcfs` 的文件 reader 会在返回 bytes 时写同一类 `file.read` 文件访问证据。其它宿主投影必须走等价的 `observability.Recorder` 接缝。

## 系统性检索证据模型

检索观测不是一种日志覆盖所有阶段，也不是提前发明 Logical Retrieval Program。当前实现只记录各原语
已经真实产生的 source facts，并用稳定 ID 串成因果链：

```text
access ev_*                   身份、授权、固定知识 basis、Canonical READ
  └─ retrieval rt_*           SEARCH / RELATION 的逻辑请求与返回候选窗
       └─ refine rf_*          可选语义 filter/rerank 的精确模型输入输出
            └─ feedback        Agent 答案、引用、用户确认或纠正
                 └─ training  可重建且带标签强度的 retrieval/rerank 样本
```

各类原始证据职责如下：

| 证据 | 覆盖范围 | 保存内容 |
|---|---|---|
| `access.jsonl` | READ、SEARCH、RELATION、RERANK 等知识消费 | principal/onBehalfOf、授权、固定 commit/Address、结果或错误 |
| `retrieval.jsonl` | 当前 `SEARCH` 与一跳 `RELATION` 原语 | logical request/digest、SearchView、最终候选顺序、provider/lane/local rank、Canonical value digest、State observations、候选/回读/丢弃计数和阶段耗时、完整性与错误 |
| `refine.jsonl` | 可选 semantic refine | 精确模型可见投影、模型与 prompt revision、完整结构化输出或失败 |
| `feedback.jsonl` | 对 retrieval/refine 的监督 | Agent 最终答案、实际引用、用户接受/纠正与提交 trace |

`retrieval.jsonl` 不复制完整知识正文。Snapshot 候选通过 pinned KnowledgeRef 回读；记录 value digest 用于
检测离线数据提取是否仍对应原结果。动态 State 候选同时保存 observation basis。opaque continuation 只记
输入/输出是否存在，不保存 PIT 等物理 provider cursor。当前引擎尚未暴露的候选（例如 residual filter
丢弃对象的身份）只记数量，不能从统计反推或虚构 stage。

SEARCH 和 RELATION 成功响应返回 `retrievalEvidenceId`。`search:rerank` 的 refine evidence 通过
`retrievalEvidenceId` 指向上游候选窗；下游 refine 失败不会把已完成的 retrieval 标成失败。原始查询使用：

- `POST /operations/v1/retrieval-log:query`：按 evidenceId、trace、operator、provider、outcome 查询；
- `POST /operations/v1/retrieval-training:query`：从 retrieval + feedback 重建训练样本；
- `POST /operations/v1/refine-log:query` 与 `rerank-training:query`：对应语义 refine。

反馈可直接引用 retrieval evidence，也可只引用 refine evidence；后者若存在上游 retrieval link，服务会
自动传播 join key，使一次用户确认同时可用于评估召回候选窗和训练语义重排。候选引用在对应窗口内
强校验，模型输出本身永远不是监督真值。

未来 lexical/sparse/dense/late-interaction/fusion 原语稳定后，应在同一 `rt_*` 事件中增加版本化 stage
source facts；不要把尚未执行的计划伪装成观测结果。精确 READ 本身不是候选检索，继续只归 access evidence。

## Semantic Refine evidence 与训练数据闭环

LLM rerank 的输入输出不是普通运行日志，也不能只记一个 model/latency 指标。每次实际进入语义
Provider 的调用都会在 access evidence 成功落盘后，追加一条独立的 `.kc/refine.jsonl`：

```text
access evidence ev_*                    谁在什么固定 basis 读了哪些知识
        ↓ accessEvidenceId
retrieval evidence rt_*                 查询、候选窗、lane、hydrate 与完整性
        ↓ retrievalEvidenceId（search:rerank 时）
refine evidence rf_*                    问题、模型可见候选、来源 rank、模型完整输出
        ↓ refineEvidenceId + traceId
feedback: answered                      Agent 最终答案及实际使用的候选
feedback: accepted/corrected/...        用户或评审反馈
        ↓ rebuildable join
rerank training sample                  带 labelStrength/trainingEligible 的派生视图
```

Refine evidence 保存：

- `schemaVersion`、`rf_*`、对应 `ev_*`、identity/trace/request/workspace；
- SearchView 的 Snapshot 与动态 projection revision；
- 冻结的 semantic spec：criterion、projection、topK/ties/unjudged；
- `search:rerank` 的结构化 RetrievalQuery；
- 每个候选的 pinned KnowledgeRef、**实际发给模型的投影值**及 digest、State observation basis、
  originalRank 与 provider/lane/localRank/localScore/matchedFields；
- Provider/model/modelRevision/promptRevision、完整 pre-topK RankGroups/unjudged、耗时；
- Provider 超时、非法输出等失败的稳定错误码。只要投影输入已经形成，失败调用也记录。

当 Provider 输出未知、重复或遗漏候选时，原始结构化输出与校验错误同时保留，便于区分模型质量和
transport 故障；这类失败记录永远不进入可训练样本。

明确不保存：API key、Authorization、HTTP transport body、未被 `EvaluationProjection` 选中的知识字段、
模型隐式推理或 chain-of-thought。这里记录的是精确模型输入，而不是 Canonical 知识副本；它仍可能
包含业务敏感内容，因此只有 `audit.read` 可查询，部署者必须像保护 access/feedback 一样保护 KC Home。
成功 rerank 在 refine append/fsync 失败时不得向调用方返回成功。

成功响应的 `evidence.refineEvidenceId` 是后续监督信号的稳定 join key。Agent 完成答案后提交：

```json
{
  "workspace": "finance-board",
  "traceId": "trace-42",
  "outcome": "answered",
  "refineEvidenceId": "rf_...",
  "answer": "最终给用户的答案",
  "selectedRefs": [{"repository": "kr://...", "object": "runbook/refund"}]
}
```

用户接受可再写一条 `accepted`；纠正可携带 `selectedRefs` 或 `idealGroups`。所有监督 Ref 必须来自
对应候选窗，防止把另一 basis 的对象误贴为标签。`labelSource` 由认证主体推导，Agent 不能自称用户。
反馈提交通常是稍后的新请求：`trace` 保留被评价的 Agent trace，`submissionTrace` 单独保存反馈请求自身
的调用坐标，二者不会互相覆盖；trace 查询从任一坐标都能定位这条反馈。

训练视图通过 `POST /operations/v1/rerank-training:query` 从 `refine.jsonl + feedback.jsonl` 重建：

- Agent 的 `answered + selectedRefs` 单独只是 `agent-weak`，默认不标记可训练；
- 同一 evidence 只有一个带候选引用的 Agent 答案，并且后续得到用户/人工评审的 `accepted/helpful`，
  才成为 `accepted-answer`；Agent 自评或多个答案无法确定被确认的是哪一个时仍是弱标签；
- 用户/人工评审给出明确候选纠正的 `corrected` 成为 `corrected`；
- 单纯模型输出、无引用答案或只有 rejected/unhelpful 不会自动变成监督真值。

原始记录查询使用 `POST /operations/v1/refine-log:query`，可按 evidenceId、trace、provider、model、
outcome 过滤；指定 limit 时返回最新的匹配窗口且保持时间顺序。训练样本仍是非 Canonical 派生数据；微调、去标识化、质量抽检、数据集版本和删除策略
属于墙外训练流水线，不反向写知识仓，也不能未经审查把 Agent 自评当成人类标签。当前参考实现不
自动过期或上传这些记录，避免静默丢失；生产部署仍需补充组织级 retention/erasure/encryption 策略。

## Hitmap

`kc operations audit hitmap` 从成功的 `ALLOW + RESOLVED` 访问事件实时派生，不单独维护权威计数。分组键是：

```text
repository + commit + object + Address
```

因此同一对象的两个版本是两条 hit，不会把“访问过 GMV”模糊成未版本化统计。结果包含次数、首次/最后访问时间，以及按 `principal`、`onBehalfOf` 的计数。删除 `.kc/access.jsonl` 后 hitmap 可重建为空；它从不反向影响授权、搜索排序或知识内容。
