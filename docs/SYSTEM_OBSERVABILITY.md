# 系统可观测性规范

日期：2026-08-31

定位：metric/log/trace、健康与 SLI/SLO 的规范草案；实现状态只在
`MVP_ACCEPTANCE.md` / `TEST_CATALOG.md` 维护。

本文定义 Knowledge Catalog 运行系统怎样发现故障、解释性能、衡量可靠性，并把一次请求关联到已有的知识访问证据。它不改变 ⓪–③ 协议分层，也不把 metric、diagnostic log 或 distributed trace 写成知识。

术语以 [`TERMINOLOGY.md`](TERMINOLOGY.md) 为准；知识访问证据以 [`OBSERVABILITY.md`](OBSERVABILITY.md) 为准。本文中的“必须/不得”表示不可违反的合同，“应当/不应当”表示除非有明确记录的理由否则遵守，“可以”表示可选，含义与 BCP 14（RFC 2119/RFC 8174）一致。

---

## 0. 从目标到信号的方法

可观测性不以“指标数量”或“面板数量”为完成标准。每个信号必须服务于至少一个明确目标：

1. **当前状态**：用户的关键旅程现在是否可用、快速、完整、新鲜且可审计。
2. **提前发现**：资源、队列、fan-out、写放大或基数是否接近容量边界。
3. **可执行告警**：用户已经受损或 error budget 正在快速消耗时，通知能采取行动的人。
4. **故障定位**：从受损旅程下钻到阶段、依赖、资源、单次 trace 和受控日志。
5. **容量与演进**：解释负载、数据量、延迟、成本和退化斜率之间的关系。
6. **身份与行为**：在不泄露身份、不制造 metric 高基数的前提下，分析采用、委托、拒绝、异常和知识使用。

### 0.1 组合方法

本系统使用一条固定的设计链，而不把 RED、USE 或“三支柱”当成相互竞争的方法：

```text
用户/治理承诺
  → good event / valid event SLI
  → 症状：Rate + Errors + Duration + Quality/Freshness/Durability
  → 原因：每个可穷尽资源的 Utilization + Saturation + Errors
  → 证据：metric 聚合 → trace 阶段 → log/event 细节 → 版本化访问证据
  → SLO / error budget / 多窗口 burn-rate 告警
  → runbook、演练和容量计划
```

- **Golden Signals / RED** 放在用户和 Surface 边界，回答“什么坏了”。本项目还必须纳入通用 RED 没有表达的 completeness、freshness、durability 和 correctness。
- **USE** 放在 CPU、memory、FD、connection pool、queue、authority、projection 和 telemetry pipeline 边界，回答“为什么坏”以及“是否即将耗尽”。
- **trace/log** 是诊断下钻，不是 SLI 或审计账。**access/audit evidence** 是身份和固定 basis 的可信事实，不受采样的 telemetry 代替。
- **黑盒观测**证明关键旅程真的可用；**白盒观测**揭示被重试遮住的失败、内部瓶颈和即将发生的容量问题。两者都必须有，不用 readiness 代替用户旅程。

### 0.2 覆盖单元

每个关键旅程或高风险组件必须在设计和评审时填满下列要素；缺任一项都不算“已可观测”：

| 要素 | 必须回答的问题 |
|---|---|
| 承诺与用户 | 谁依赖它，哪个结果才算 good event？ |
| 失败模式 | 不可用、慢、partial、stale、错误、丢证据分别如何表现？ |
| 容量模型 | 哪个输入量会使工作量放大，哪个资源先饱和？ |
| 原始信号 | 需要哪些 counter、histogram、gauge、event 和 span？ |
| 聚合与目标 | SLI 的 numerator/denominator、窗口、阈值和最小流量是什么？ |
| 诊断下钻 | 告警后先分哪个阶段/依赖，再用哪个 trace/log/evidence 定位？ |
| 行动与所有者 | 是 page、ticket、安全审查还是趋势；收到后能做什么？ |
| 验证 | 哪个负载、故障注入或端到端用例证明数据和告警真的有效？ |

指标命名或面板视觉上完整，但没有 good/valid event、无法导向行动或没有失败注入证据时，仍必须登记为 gap。

### 0.3 告警输出分层

| 输出 | 触发原则 | 典型信号 |
|---|---|---|
| Page | 正在发生显著用户损害，或有足够高精度的短期耗尽风险 | availability/latency/quality 多窗口 burn；evidence fail-closed；大面积 readiness |
| Ticket | 暂不需立即介入，但持续会伤害 SLO 或容量 | projection lag、queue saturation、依赖退化、telemetry drop、容量预测 |
| Security review | 高基数身份事件的规则或基线异常 | 拒绝激增、异常代理、罕见仓访问、写爆发 |
| Dashboard/report | 当前理解、趋势、基线和采用，无须通知 | 流量、分位数、用户聚合、存储增长、版本对比 |

不得因为某个原因指标“看起来异常”就 page；page 优先绑定用户症状或明确即将耗尽的有限资源。

---

## 1. 信号与失败边界

系统保留四类用途不同的数据，不得混成一条“日志”：

| 数据 | 回答的问题 | 可靠性 | 权威性 |
|---|---|---|---|
| Canonical 与 git 历史 | 知识和治理状态是什么、怎样变化 | 由 Snapshot/Registry 保证 | 协议权威 |
| access / feedback / system / audit 证据 | 谁以哪个固定版本做了什么 | 不采样；按各 Surface 的既有语义持久化 | 非 Canonical 的审计证据 |
| metric / diagnostic log / distributed trace | 系统是否健康、慢在哪里、为什么失败 | 可采样；异步导出；允许受控丢失 | 诊断遥测，不作授权或审计依据 |
| dashboard / alert / SLO report | 当前是否需要人介入 | 可从遥测重建 | 派生视图 |

失败边界：

1. 必须记录 access evidence 的成功消费，只有 evidence 持久化后才能向调用方交付成功；持久化失败时返回原有错误信封，不能声称“成功且可审计”。
2. OTLP、Prometheus 或诊断日志导出失败不得改变 READ、SEARCH、COMMIT、Catalog 操作的协议结果，也不得覆盖原始业务错误。
3. telemetry 不得进入 Repository、Catalog pin、provenance、索引排序或权限判断。
4. access evidence 不得由 span、metric、stdout log 或采样策略替代。
5. telemetry header 非法、冲突或缺失属于诊断上下文问题，不得成为业务请求失败的原因。

### 1.1 关键旅程

| 旅程 | good event | 另行观测的非 good 结果 |
|---|---|---|
| Resolve Workspace | 在同一命令内一次解得授权 Repository 的固定 pin | unresolved、denied、仓级技术错误、fan-out 退化 |
| Canonical READ | 从已解开的 commit 返回完整值；“真的不存在”也是可靠结果 | authority I/O 失败、Schema/单元组装错误、慢读 |
| SEARCH | 查询能力被满足，projection 与 Canonical hydrate 在同一 basis 上返回 complete 结果 | partial 原因、零结果、candidate 放大、provider/hydrate 失败 |
| Writer COMMIT/PROPOSAL | 命令被 committed 或按同 digest replayed，且回执持久化 | conflict、Schema 拒绝、CAS/authority/command-log 技术失败 |
| Publish → searchable | 目标 commit 已成为预期 head，投影在 freshness objective 内 READY | backlog、rebuild 失败、旧 generation 继续服务、错误空结果 |
| Bound State READ/SEARCH | 声明 commit 和 observation basis 一致，在指定 freshness 内 hydrate | timeout、stale observation、generation 不匹配、动态 coverage 不完整 |
| Audited delivery | 成功交付之前 access evidence 已 append + fsync，可用 `evidenceId` 对账 | append 失败、覆盖缺口、记录增长与磁盘饱和 |
| Workspace file read | 按固定 pin 返回正确 bytes 并记录 file access evidence | page/read fan-out、mount 连接、authority 超时、evidence 失败 |

设计 SLO 时先从这些旅程选不超过五类最重要的 SLI；组件指标只用于解释它们。

### 1.2 故障与容量风险地图

| 边界 | 最容易出问题的地方 | 规模放大因子 | 必须具备的症状与原因信号 |
|---|---|---|---|
| HTTP/身份 | 认证方不可用、token 校验慢、拒绝激增、伪造委托 | RPS、认证提供方延迟、active requests | HTTP RED；authn/authz outcome 与延迟；provider CLIENT span；`ruleId` 和可信身份事件 |
| Catalog/Workspace | selector 解析失败、成员仓慢/无权、一次任务 pin 不一致 | Repository 成员数 × resolve_ref 往返 | resolve good ratio/延迟；member histogram；仓级 outcome 聚合；resolve_ref span |
| Snapshot authority | Gitea/Dolt I/O、连接池、CAS、大 tree/page、历史退化 | 读次数、bytes、page size、commit 深度、并发 | operation rate/error/duration/in-flight/bytes；store 资源 USE；CLIENT span |
| Canonical Reader | Aspect/member 组装放大、ReadMany 退化为 N 次远程读 | object 数 × units/object × Repository 数 | READ SLI；单次请求的 object/unit/authority-call 聚合数；snapshot child spans |
| SEARCH | provider 慢、query lane 放大、候选过多、Canonical hydrate 慢、partial 被当成成功 | Repository × clauses × candidates，再加 hydrated object/unit 回读 | availability/completeness/latency；plan/probe/hydrate；candidate amplification/drop；provider + authority spans |
| Writer | 大 ChangeSet、Schema 验证、command log/fsync、CAS 争用、长历史 | PUT/REMOVE 数、canonical bytes、并发 writer、commit 深度 | availability/latency；conflict 与 technical error 分开；change count/bytes；validate/CAS/ledger spans |
| Projection | outbox 丢失或堆积、rebuild 追不上、bulk 失败、generation 切换 | change rate − apply throughput；docs/change；docs 总量 | freshness/coverage；queue depth/oldest age；docs/s、batch error、rebuild ETA；state transition event |
| Binding runtime | 远程 timeout、source revision 落后、观测基础不匹配、下游连接饱和 | bound units × hydrate fan-out；外部查询成本 | lookup good ratio/延迟；observation age；provider CLIENT/SERVER span；bounded error |
| Evidence/journal | fsync 慢、磁盘满、单条命中太多、retention 无界 | audited RPS × event bytes × retention | durability/coverage 与 append latency/bytes；disk USE；`evidenceId` reconciliation |
| Hook/Gate/Control | 出站时间过长、outbox 堆积、gate 证据缺失、重试风暴 | hooks/operation、pending events、external latency | dispatch/check outcome + duration；outbox depth/age；CLIENT span；不用 hook 失败覆盖原操作结果 |
| VFS/kcfs | mount 中断、page fan-out、大文件、证据 append 拖慢读 | lookup/read RPS、bytes、directory entries | file journey RED；bytes/page entries；Gateway→authority spans；mount process USE |
| 进程/宿主 | CPU/GC、heap/RSS、goroutine、FD、network/disk 饱和 | 并发、负载形状、大 payload、高基数 telemetry | 每个资源的 utilization/saturation/errors；与 operation active/rate 对齐 |
| Telemetry pipeline | SDK/Collector 队列满、backend 慢/不可用、采样错、标签爆炸 | spans/logs per request、series cardinality、retention | accepted/refused/enqueue-failed/send-failed；queue size/capacity；backend ingest/query health；端到端 canary |

容量面板必须同时显示“输入负载→工作量放大→饱和/队列→用户延迟”，否则只能看到结果，不能用于规模规划。

---

## 2. 关联与传播

### 2.1 标识所有权

| 标识 | 语义 | 基数与去向 |
|---|---|---|
| `requestId` | 一次传输或命令调用 | response header、trace、受控日志、过程账 |
| `traceId` / `spanId` | W3C/OTel 调用图坐标 | trace、受控日志、过程账 |
| `pinId` | 一次消费冻结的 ResolvedWorkspace 标识 | access evidence、trace、受控日志 |
| `commandId` | 一次可重放写命令 | Writer receipt、trace、受控日志 |
| `evidenceId` | Recorder 为一条已持久化 evidence 生成的唯一标识 | evidence、内部 delivery ack、受控过程账 |

`pinId` 和 `commandId` 是业务一致性坐标；`requestId` 和 trace/span 是调用关联坐标，
互相不得替代。跨多次 KC 调用的一项 Agent 任务直接使用同一 trace，不再增加独立的任务会话标识。

调用方提供的 `requestId` 可能重复，不能单独证明一次成功交付拥有 evidence。
`evidenceId` 必须由 Recorder 生成且不得接受调用方输入；Recorder 只有在持久化完成后，
才把它作为内部 ack 返回给 response boundary。“持久化完成”至少包含完整单行写入、文件
`fsync` 和 close 成功，不能把仅进入进程页缓存视为 delivery ack。

### 2.2 HTTP 传播

服务入口必须支持 W3C `traceparent`，应当透传合法 `tracestate`：

1. 有合法 `traceparent` 时，以它为父上下文。
2. 没有 `traceparent` 时，可以把旧 `X-Kc-Trace-Id` 系列转换为父上下文；无法无损转换时建立新的 root trace。
3. 两套头同时出现时，`traceparent` 胜出，旧头被忽略；记录 `kc.propagation.outcome=conflict`。
4. `traceparent` 或 `tracestate` 非法时，丢弃非法部分并建立/继续合法上下文；记录 `kc.propagation.outcome=invalid`，不得拒绝业务请求。
5. 出站远程调用必须写入当前 `traceparent`；不得传播 `principal`、`onBehalfOf` 或 token 到 W3C baggage。

缺少 `X-Kc-Request-Id` 时，入口必须生成不超过 128 字符的 correlation token，并通过响应头 `X-Kc-Request-Id` 返回。错误正文继续保持统一形状 `{error:{code,message}}`，不得为了诊断追加不兼容字段。

CLI 为每次命令建立 root span；HTTP 使用 server span。CLI/HTTP 必须把最终 request/trace
上下文写入同一次 access evidence 和 system/audit journal。

---

## 3. Trace、属性与日志

### 3.1 Span 边界

HTTP transport 必须使用 OpenTelemetry HTTP semantic conventions：server span kind 为 `SERVER`，span name 使用低基数 method + route template；不得自造 `kc.http.server` 代替标准 HTTP span。CLI root span kind 为 `INTERNAL`，命名为 `kc <verb>`。

应用层子 span 只围绕具有独立失败或延迟价值的边界：

```text
HTTP SERVER span / kc <verb>
└── kc.invoke <verb>
    ├── kc.authenticate
    ├── kc.authorize
    ├── kc.catalog.resolve_workspace
    │   └── kc.snapshot.resolve_ref
    ├── kc.knowledge.read
    │   └── kc.snapshot.read
    ├── kc.retrieval.search
    │   ├── kc.retrieval.probe
    │   ├── kc.retrieval.candidates
    │   └── kc.retrieval.hydrate
    ├── kc.writer.commit / kc.writer.proposal
    │   └── kc.snapshot.compare_and_swap
    ├── kc.projection.apply / kc.projection.rebuild
    ├── kc.hook.dispatch / kc.gate.check
    └── kc.evidence.append
```

- 出站 Gitea/OpenSearch/Hook HTTP 请求使用标准 `CLIENT` span，再补 KC 领域属性。
- 不得为每个 search hit、Aspect member 或 tree entry 建 span；父 span 记录聚合数量和至多一个摘要 event。
- span 名不得拼 Repository、object、path、commit、principal、错误消息或其它自由文本。
- 一次 Workspace 消费的所有子 span 必须使用同一个 `pinId`，不得重新跟随 selector。
- SEARCH span 必须记录 completeness、候选数、回读数、丢弃数、continuation 是否产生，以及有界的 partial reason；provider 查询成功不得等同于知识结果 complete。

### 3.2 稳定属性词表

所有跨 Signal 的通用 attribute 必须来自下表；未知枚举统一映射为 `other`，不得把原始值降级成 label。单个 instrument 的专用 attribute 还必须满足第 4.1 节的值域。

| 属性 | 允许值/来源 | Signal |
|---|---|---|
| `kc.face` | `catalog|knowledge|writer|projection|vfs|control|hook|gate|other` | metric、span、log |
| `kc.operation` | CLI 命令表或内部稳定操作表 | metric、span、log |
| `kc.outcome` | `ok|partial|unresolved|denied|invalid|conflict|error` | metric、span、log |
| `error.type` | 失败时的稳定 kernel code；未知技术错误为 `other` | metric、span、log |
| `kc.snapshot.store` | `gitea|dolt|other` | metric、span、log |
| `kc.retrieval.provider` | `none|opensearch|other` | metric、span、log |
| `kc.search.completeness` | `complete|partial` | metric、span、log |
| `kc.search.partial_reason` | `authorization|unsupported|projection|hydrate|binding|other` | metric、span、log |
| `kc.projection.state` | `BUILDING|READY|UPDATING|FAILED|RETIRED|other` | metric、span、log |
| `kc.propagation.outcome` | `accepted|generated|legacy|invalid|conflict` | metric、span、log |
| `kc.principal.kind` | `owner|user|agent|service|other` | span、log |
| `kc.identity.provider` | `local|gitea|oidc|taihu|other` | metric、span、log |
| `kc.binding.mode` | `state|stream|other` | metric、span、log |

instrument 专用 attribute 的稳定值域：

| 属性 | 允许值/来源 |
|---|---|
| `kc.authorization.decision` | `allow|deny` |
| `kc.identity.delegated` | boolean；只表示已验证身份是否包含委托，不携带主体值 |
| `kc.writer.surface` | `COMMIT|PROPOSAL|other` |
| `kc.writer.replayed` | boolean |
| `kc.writer.change.operation` | `PUT|REMOVE|other` |
| `kc.projection.mode` | `incremental|rebuild|ready|other` |
| `kc.projection.cause` | `content|schema|ready|cold|diverged|other` |
| `kc.projection.change.operation` | `update|remove` |
| `kc.projection.from_state` / `kc.projection.to_state` | 与 `kc.projection.state` 相同 |
| `kc.hook.phase` | `pre|post|other` |
| `kc.hook.transport` | `exec|http|outbox|other` |
| `kc.outbox.kind` | `projection|hook|other` |
| `kc.gate.required` | boolean；当前 merge 是否声明至少一项 gate requirement |
| `kc.evidence.kind` | `access|feedback|system|audit|other` |
| `kc.telemetry.signal` | `metric|log|trace|other` |
| `kc.telemetry.drop_reason` | `queue_full|timeout|export_error|shutdown|other` |

映射规则：

- 成功且完整 → `ok`；合法 partial SearchResult → `partial`。
- `*_UNRESOLVED` → `unresolved`；`UNAUTHENTICATED` / `FORBIDDEN` → `denied`。
- `USAGE_INVALID` / 调用方 `PRECONDITION_FAILED` → `invalid`。
- `NON_FAST_FORWARD` / `IDEMPOTENCY_CONFLICT` → `conflict`。
- `TEMPORARY_UNAVAILABLE`、I/O、超时、内部不变量失败 → `error`。
- `error.type` 只在非 `ok`/`partial` outcome 上出现；不得用错误消息作属性值。

Repository、commit、object、Address、path、principal、onBehalfOf、requestId、traceId、
pinId、commandId、evidenceId 均为高基数，不得成为 metric attribute。它们只可以进入 access
evidence、受限 span 或受限 diagnostic log；正文敏感的 object/path 应当按部署策略省略或盐化摘要。

### 3.3 Diagnostic log

诊断日志采用 OTel Log Data Model。service 身份是 Resource attributes，不在每条日志里重复发明嵌套对象：

```json
{
  "timestamp": "...",
  "severity_text": "INFO",
  "body": "kc.http.request.completed",
  "trace_id": "...",
  "span_id": "...",
  "attributes": {
    "kc.request.id": "...",
    "kc.outcome": "ok",
    "kc.duration_ms": 84,
    "http.request.method": "POST",
    "http.route": "/knowledge/v1/{operation}",
    "http.response.status_code": 200
  }
}
```

Resource 必须设置 `service.namespace=knowledge-catalog`、`service.name`、`service.version`、全局唯一的 `service.instance.id` 和 `kc.telemetry.schema.version`。正常边界调用至多写一条 completion log；内部只在重试、状态迁移、降级和异常时追加 event。

任何 log/span event/metric 不得包含知识正文、ChangeSet value、Authorization/Cookie/token、Binding secret、完整 query、完整 argv 或未脱敏外部响应。查询排障只记录 query shape、clause 数和可选盐化摘要。

现有 `.kc/system.jsonl` 与 `.kc/audit.jsonl` 继续是 pointers-only 本机过程账，不因接入 stdout/OTLP 日志而改名或降级；两者可以共享 request/trace 关联字段，但存储、访问控制和 retention 独立。

---

## 4. Metrics 合同

### 4.1 两种命名层

OTel instrument name 是代码和 OTLP 的规范名称；Prometheus exposition name 是 exporter 映射结果。两者不得在代码中各建一套重复 instrument。HTTP transport 直接使用 OTel 标准 `http.server.request.duration` 和 `http.server.active_requests`，KC 指标只描述应用操作。

| OTel instrument | 类型 | Unit | Prometheus exposition | 允许属性 |
|---|---|---|---|---|
| `kc.operation.executions` | Counter | `{operation}` | `kc_operation_executions_total` | `kc.face`、`kc.operation`、`kc.outcome`、`error.type` |
| `kc.operation.duration` | Histogram | `s` | `kc_operation_duration_seconds` | `kc.face`、`kc.operation`、`kc.outcome` |
| `kc.operation.active` | UpDownCounter | `{operation}` | `kc_operation_active` | `kc.face`、`kc.operation` |
| `kc.authentication.attempts` | Counter | `{attempt}` | `kc_authentication_attempts_total` | `kc.identity.provider`、`kc.outcome`、`error.type` |
| `kc.authentication.duration` | Histogram | `s` | `kc_authentication_duration_seconds` | `kc.identity.provider`、`kc.outcome` |
| `kc.authorization.decisions` | Counter | `{decision}` | `kc_authorization_decisions_total` | `kc.operation`、`kc.authorization.decision` |
| `kc.identity.requests` | Counter | `{request}` | `kc_identity_requests_total` | `kc.identity.provider`、`kc.principal.kind`、`kc.identity.delegated` |
| `kc.workspace.resolve.duration` | Histogram | `s` | `kc_workspace_resolve_duration_seconds` | `kc.outcome` |
| `kc.workspace.member.count` | Histogram | `{repository}` | `kc_workspace_member_count` | `kc.outcome` |
| `kc.snapshot.operations` | Counter | `{operation}` | `kc_snapshot_operations_total` | `kc.snapshot.store`、`kc.operation`、`kc.outcome`、`error.type` |
| `kc.snapshot.operation.duration` | Histogram | `s` | `kc_snapshot_operation_duration_seconds` | `kc.snapshot.store`、`kc.operation`、`kc.outcome` |
| `kc.snapshot.operation.active` | UpDownCounter | `{operation}` | `kc_snapshot_operation_active` | `kc.snapshot.store`、`kc.operation` |
| `kc.snapshot.operation.bytes` | Histogram | `By` | `kc_snapshot_operation_bytes` | `kc.snapshot.store`、`kc.operation`、`kc.outcome` |
| `kc.search.requests` | Counter | `{request}` | `kc_search_requests_total` | `kc.retrieval.provider`、`kc.search.completeness`、`kc.search.partial_reason`、`kc.outcome` |
| `kc.search.duration` | Histogram | `s` | `kc_search_duration_seconds` | `kc.retrieval.provider`、`kc.search.completeness`、`kc.outcome` |
| `kc.search.phase.duration` | Histogram | `s` | `kc_search_phase_duration_seconds` | `kc.retrieval.provider`、`kc.search.completeness`、`kc.outcome`、`kc.search.phase=plan|probe|hydrate|orchestration` |
| `kc.search.candidate.count` | Histogram | `{candidate}` | `kc_search_candidate_count` | `kc.retrieval.provider` |
| `kc.search.hydrated.count` | Histogram | `{object}` | `kc_search_hydrated_count` | `kc.retrieval.provider` |
| `kc.search.dropped.count` | Histogram | `{candidate}` | `kc_search_dropped_count` | `kc.retrieval.provider`、`kc.search.partial_reason` |
| `kc.writer.commands` | Counter | `{command}` | `kc_writer_commands_total` | `kc.writer.surface`、`kc.outcome`、`error.type`、`kc.writer.replayed` |
| `kc.writer.duration` | Histogram | `s` | `kc_writer_duration_seconds` | `kc.writer.surface`、`kc.outcome`、`kc.writer.replayed` |
| `kc.writer.change.count` | Histogram | `{change}` | `kc_writer_change_count` | `kc.writer.surface`、`kc.writer.change.operation` |
| `kc.writer.payload.size` | Histogram | `By` | `kc_writer_payload_size_bytes` | `kc.writer.surface`、`kc.outcome` |
| `kc.projection.transitions` | Counter | `{transition}` | `kc_projection_transitions_total` | `kc.retrieval.provider`、`kc.projection.from_state`、`kc.projection.to_state`、`kc.projection.cause` |
| `kc.projection.duration` | Histogram | `s` | `kc_projection_duration_seconds` | `kc.retrieval.provider`、`kc.projection.mode`、`kc.outcome` |
| `kc.projection.lagging.count` | ObservableGauge | `{projection}` | `kc_projection_lagging_count` | `kc.retrieval.provider` |
| `kc.projection.oldest_pending.age` | ObservableGauge | `s` | `kc_projection_oldest_pending_age_seconds` | `kc.retrieval.provider` |
| `kc.projection.documents` | ObservableGauge | `{document}` | `kc_projection_documents` | `kc.retrieval.provider` |
| `kc.projection.change.count` | Histogram | `{document}` | `kc_projection_change_count` | `kc.retrieval.provider`、`kc.projection.change.operation=update|remove` |
| `kc.binding.lookups` | Counter | `{lookup}` | `kc_binding_lookups_total` | `kc.binding.mode`、`kc.outcome`、`error.type` |
| `kc.binding.lookup.duration` | Histogram | `s` | `kc_binding_lookup_duration_seconds` | `kc.binding.mode`、`kc.outcome` |
| `kc.binding.observation.age` | Histogram | `s` | `kc_binding_observation_age_seconds` | `kc.binding.mode`、`kc.outcome` |
| `kc.evidence.appends` | Counter | `{append}` | `kc_evidence_appends_total` | `kc.evidence.kind`、`kc.outcome` |
| `kc.evidence.append.duration` | Histogram | `s` | `kc_evidence_append_duration_seconds` | `kc.evidence.kind`、`kc.outcome` |
| `kc.evidence.append.bytes` | Histogram | `By` | `kc_evidence_append_bytes` | `kc.evidence.kind`、`kc.outcome` |
| `kc.telemetry.dropped` | Counter | `{record}` | `kc_telemetry_dropped_total` | `kc.telemetry.signal`、`kc.telemetry.drop_reason` |
| `kc.hook.dispatches` | Counter | `{dispatch}` | `kc_hook_dispatches_total` | `kc.hook.phase`、`kc.hook.transport`、`kc.outcome` |
| `kc.hook.duration` | Histogram | `s` | `kc_hook_duration_seconds` | `kc.hook.phase`、`kc.hook.transport`、`kc.outcome` |
| `kc.hook.outbox.pending` | ObservableGauge | `{event}` | `kc_hook_outbox_pending` | 无 |
| `kc.hook.outbox.oldest_pending.age` | ObservableGauge | `s` | `kc_hook_outbox_oldest_pending_age_seconds` | 无 |
| `kc.gate.checks` | Counter | `{check}` | `kc_gate_checks_total` | `kc.gate.required`、`kc.outcome` |
| `kc.gate.duration` | Histogram | `s` | `kc_gate_duration_seconds` | `kc.gate.required`、`kc.outcome` |
| `kc.vfs.transfer.size` | Histogram | `By` | `kc_vfs_transfer_size_bytes` | `kc.operation`、`kc.outcome` |
| `kc.vfs.directory.entry.count` | Histogram | `{entry}` | `kc_vfs_directory_entry_count` | `kc.operation`、`kc.outcome` |

`kc.operation` 的公开动词来自 `cli/command.go`，内部操作由 telemetry 词表显式登记。未登记值映射为 `other`。
Snapshot 内部操作限于 `resolve_ref|read|read_many|list_page|history|diff|commit|compare_and_swap|other`；不得将 ref、path 或 Repository 拼入操作名。

上表是按风险模型要求的原始信号合同，不表示参考实现已覆盖每一项。实现状态只在
[`TEST_CATALOG.md`](TEST_CATALOG.md) 维护。对 Collector 和 backend 本身不重造 `kc.*` 指标：直接采集
`otelcol_receiver_accepted_*`、`otelcol_receiver_refused_*`、`otelcol_exporter_enqueue_failed_*`、
`otelcol_exporter_send_failed_*`、`otelcol_exporter_queue_size/capacity` 以及各 backend 自有的 ingest/query/storage 指标。
外部 Collector 不进核心包；每个 provider integration 必须另行给出 source event 输入、backlog、preview/commit 延迟和 source→Canonical→projection 新鲜度。

每个 instrument 的稳态 labelset 基数预算应小于 100；预计超过 100 的维度必须移到 trace/log/evidence 或离线聚合，不得以“暂时用户少”为由进入 metric label。

参考实现提供覆盖当前 reference objective 的默认 Histogram bucket；SEARCH/HTTP/operation
在 `0.75s–4s` 目标邻域至少包含 `0.75/1/1.25/1.5/2/2.5/3/4s`，避免把位于
`1s–2.5s` 宽桶中的请求插值成接近 2.5s 的误导性 P95。bucket 合同独立维护在
`internal/telemetry/metric_contract.go`。deployment profile 可通过 OTel View 覆盖，但
Dolt 与远程 Gitea/OpenSearch 不应共用一组未经基线验证的阈值。instrument 的名称、类型、unit 或属性语义发生破坏性变化时，必须提升 telemetry schema version；稳定 dashboard 使用的旧 instrument 至少跨一个发布周期双发或提供 recording-rule 迁移。

### 4.2 获取与集成验证

常驻 `kc serve` 在同一进程的 `GET /metrics` 导出 Prometheus 原始样本。所有
Catalog、Knowledge、Writer、Governance、Operations 与 Workspace File 请求，即使
来自同机 CLI、Connector 或 `kcfs`，也必须经过该 Server，因此服务指标不存在“本地
CLI 执行后丢失”的第二口径。CLI 只记录客户端调用 trace；它不执行知识操作，也不需要
metrics snapshot 命令。Trace 和 diagnostic log 分别通过标准
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`、`OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` 使用
OTLP/HTTP 导出；两者共享 Resource 和当前 span context，但不共享存储生命周期。

可重复的真实集成夹具位于 `.data/data-warehouse/observability/`，使用可选
Compose profile 启动 Prometheus、OpenTelemetry Collector、Jaeger、Loki 和 Grafana：

```bash
make dw-obs-up
make dw-obs-smoke
make dw-obs-down
```

smoke 必须同时证明 KC 与 Collector target up、真实 SEARCH 与 Canonical READ 原始 histogram/counter、
低基数身份聚合、P95/阶段、availability burn 与 latency good-event recording rules 非空、Collector queue metric 可查询、
Jaeger 能查到 `kc-server`、Loki 能查到带同一 traceId 的 completion log，
并验证 Grafana 的 Prometheus/Jaeger/Loki 数据源、系统总览、SEARCH 分析、运行时健康、
诊断日志、容量与行为五个版本化 dashboard 及其中全部 PromQL/LogQL 可解析。
Dashboard 定义位于 `.data/data-warehouse/observability/grafana/`，不得只在运行中的
Grafana 数据库或 UI 中维护。Prometheus 3
必须在 scrape config 显式请求 legacy/下划线 metric name escaping；否则它可保留
OTel 点号名并使下划线 recording rules 全部空结果。

### 4.3 Drop 的可观察性

exporter 使用有界队列和异步批量发送。队列满时优先丢普通成功 trace/debug log，并递增本机累计 `kc.telemetry.dropped`；不得阻塞协议路径。

Collector 完全不可达时不能承诺远端立即看见 drop metric。实现必须同时：

- 保留进程内累计值，恢复后继续导出；
- 对持续 drop 发出限速 stderr diagnostic event；
- 从 Collector/agent 外部健康指标监控出口本身。

---

## 5. 健康与组件状态

| 入口 | 语义 | 规则 |
|---|---|---|
| `/livez` | 进程能否继续执行 | 只查进程/事件循环，不探远端依赖 |
| `/readyz` | 当前实例启用的全部 Surface 是否可安全承接流量 | 是各启用 Surface readiness 的 AND |
| `/readyz/{surface}` | consumer/writer/search 等分面 readiness | 供拆分部署、诊断和路由使用 |
| `/health` | 向后兼容摘要 | 汇总 live/ready，不作为深度依赖探针 |

Catalog 当前态、ControlPlane 状态和运营证据分别从正式 Catalog、Governance、Operations API 读取；不存在聚合泄漏本机 Home 的 `_state`/`_blob` 工作台端点。`/metrics` 位于独立 management listener，或受网络和管理员权限保护；这些端点不进入领域 API。

readiness 规则：

- consumer 必须能读取 Catalog/Home 控制状态并持久化 access evidence；不得用向正式 evidence 文件追加伪事件的方式探测。
- writer 必须能读取 command log 和本机必要控制状态；具体目标 Repository 的远端故障是 route health，不使无关 Repository 全局 unready。
- search 的 provider/projection 故障进入 component health；只要 API 能诚实返回支持的 partial/claims，就不自动使 Canonical READ unready。
- evidence 目录的持久化探针可以使用独立、限速的临时 probe file，并必须 create→fsync→remove；实际 append 失败立即使相应 Surface not ready。
- 昂贵远端深探针不得在每次 `/readyz` 调用时执行；依赖状态来自真实请求、按本地配置变化立即失效的短 TTL 探针缓存，或有退避的后台检查。

公开探针只返回 `status` 和稳定 `reasonCode`。受保护的 component view 至少包含 `state`、`lastCheckedAt`、`lastSuccessAt`、`lastErrorCode`、`consecutiveFailures`，不得暴露 home 路径、凭证、内部 endpoint 或原始错误正文。

---

## 6. SLI、SLO 与告警

### 6.1 规范 SLI

所有公式必须使用同一统计窗口；`technical_error` 指 `kc.outcome=error`。请求 Surface 从完成事件计数，Evidence 和 projection 使用各自的权威运行信号：

| SLI | Source | Numerator | Denominator |
|---|---|---|---|
| Canonical READ availability | `kc.operation.executions`，READ 操作集 | `ok + unresolved` | `ok + unresolved + technical_error` |
| SEARCH availability | `kc.search.requests` | `ok(complete) + partial` | `ok(complete) + partial + technical_error` |
| Writer availability | `kc.writer.commands` | `ok`（含 committed/replayed） | `ok + technical_error` |
| Evidence durability | `kc.evidence.appends` | append `ok` | append `ok + error` |
| Evidence coverage | delivery ack ↔ access evidence 按 `evidenceId` 对账 | 已持久化 evidence 的 delivered-success request | 全部 delivered-success request |
| Projection freshness | projection state + outbox age | 在 freshness objective 内达到目标 head 的 projection | 全部启用 projection |

`denied`、`invalid`、`conflict` 不进入 availability denominator，另报比率；不得借此隐藏 `error`。SEARCH completeness 只在“当前授权没有裁剪、所有 query lane 已声明支持、目标 projection 预期 READY”的 eligible search 中计算：`complete / (complete + partial)`。其它 partial 必须按 `partial_reason` 单独报告，不混入 completeness SLO。

当窗口内没有 valid event 时，比率和 burn-rate 必须保持无样本/NaN，不能把分母钳成极小正数后显示为 0% availability 或虚假的高 burn。低流量时由最小事件门槛、readiness 和独立黑盒 canary 补位。

Evidence coverage 通过 Recorder 返回的 `evidenceId` 对 delivered-success 与持久 evidence
做周期性对账；`requestId` 只用于排障关联，不能作为唯一 join key。因为成功响应必须晚于
evidence append ack，其目标恒为 100%。Evidence durability 衡量写入设施本身，不能用
fail-closed 后的响应成功率代替。

Projection freshness 从目标 head 被观察或 outbox event 入队开始，到 READY basis 等于该 head 为止；至少记录最老未处理事件年龄。没有持久 outbox 时不得宣称拥有跨进程 projection freshness SLO。

初始目标属于 deployment policy，不是协议常量。首个共享部署按 30 天窗口使用以下 reference objective，再用真实基线校准：

| 系统目标 | Reference objective |
|---|---:|
| Canonical READ availability | ≥99.9% |
| SEARCH availability | ≥99.5% |
| Writer availability | ≥99.5% |
| eligible SEARCH completeness | ≥99% |
| Evidence durability | ≥99.99% |
| Evidence coverage | 100% |
| Projection freshness | ≥99% 在 5 分钟内追上目标 head |
| telemetry dropped | 稳态为 0 |

首个 `shared-standard` 部署还使用下列单次旅程时间预算。它们是产品交互的起始承诺，不是从当前小数据 smoke 反推出的“已达到能力”：

| 旅程 | 受控工作负载 | P95 | P99 |
|---|---|---:|---:|
| Resolve Workspace | ≤8 Repository，warm authority | 500ms | 2s |
| Canonical READ | 单 object，≤6 units，已固定 commit | 500ms | 2s |
| complete SEARCH | ≤8 Repository，limit≤50，OpenSearch + Canonical hydrate | 1s | 3s |
| Writer COMMIT/PROPOSAL | ≤100 changes，不含外部 hook 人为等待 | 2s | 5s |
| Bound State READ/SEARCH | 上述读/检索且包含外部 State hydrate | 2s | 5s |
| Workspace file READ | ≤1MiB 文件，已固定 commit | 500ms | 2s |
| Evidence append | 单次已展开 event，包含 fsync | 100ms | 500ms |

时间预算不取代超时、数据量上限或容量压测。超出表中工作负载的请求必须另分 profile，不能与标准旅程混合后用一个分位数解释。

SEARCH 延迟先采用以下 reference objective。统计对象只包含 `outcome=ok` 且
`completeness=complete` 的完整请求；拒绝、输入错误和 partial 另行统计，不能混入延迟分位数：

| Profile | 受控工作负载 | P95 | P99 | slow-trace threshold |
|---|---|---:|---:|---:|
| `local-reference` | warm process/authority；单 Repository；≤10k eligible docs；limit≤20；并发≤8；本地 authority/projection；不含容器冷启动 | 250ms | 1s | 2s |
| `shared-standard` | warm process；≤8 Repository；limit≤50；网络化 authority + OpenSearch | 1s | 3s | 5s |
| `binding-state` | `shared-standard`，且包含外部 State Binding hydrate | 2s | 5s | 10s |

这里的 slow-trace threshold 是诊断阈值，不是协议超时：单次超过阈值必须保留 trace，
但一次慢请求不等于 SLO 失败。调用 deadline 属于部署/API policy，不能藏在 metric 实现中。
本地回归至少 warmup 20 次并采 200 次完整请求；共享部署在 30 天窗口或至少 10k 次完整
请求上判定。相同 profile 的 P95 相对已批准基线退化超过 25%，即使仍低于绝对目标，也应
使性能回归 gate 失败。首次真实规模基线可调整上述目标，但必须同时保存 workload、硬件、
provider、数据规模和原始 histogram，不能只提交新的阈值。

用于 error budget 的 latency SLI 必须是“阈值内的 good events / eligible events”，不是单个 P95/P99 时序：

```text
SEARCH <= 1s SLI = complete+ok 且 duration<=1s / 全部 complete+ok
SEARCH <= 3s SLI = complete+ok 且 duration<=3s / 全部 complete+ok
```

对应 reference objectives 分别为 95% 和 99%。分位数用于理解分布和长尾；阈值 bucket good ratio 用于计算 error budget 和 burn rate。

“返回了 HTTP 2xx”不能证明知识正确。Correctness 由定时黑盒 canary 证明：对专用验证 Repository 执行已知输入的 commit→READ、commit→SEARCH 和 evidence reconciliation，按预期 value/basis/visibility 相等的次数除以总次数计算。业务语义正确性仍属于 provider conformance，通用底座不能从“非空结果”推断正确。

### 6.2 规范聚合视图

应用只发 Counter、Histogram 和 Gauge 原始样本；P50/P95/P99、比率和窗口聚合由
Prometheus recording rules 派生。参考规则见
[`observability/prometheus-recording-rules.yaml`](observability/prometheus-recording-rules.yaml)，至少提供：

- 完整 SEARCH 的 P50/P95/P99，总耗时按 provider 分组；
- `plan|probe|hydrate|orchestration` 各阶段 P95；
- SEARCH/READ/Writer/Binding availability、Evidence durability；
- 标准旅程的 latency-threshold good ratio；
- partial ratio，按稳定 partial reason 分组；
- 每请求 candidates、hydrated 平均值，以及 candidate amplification、drop ratio；
- SEARCH/READ/Writer 的 30 天 error-budget remaining，以及 `5m/1h/30m/6h/2h/1d` 多窗口 burn rate；
- Writer payload/change、Projection documents/change/backlog、VFS bytes/entries、Hook outbox、认证/授权和身份委托的有界聚合。

完整 SEARCH 的主判断顺序是：先看 availability/completeness，再看总 P95/P99，然后用阶段
P95 和 candidate amplification 定位。只看平均耗时会掩盖长尾；只看总耗时无法区分 provider
查询慢、Canonical hydrate 慢或 Workspace 编排放大。

参考告警定义见
[`observability/prometheus-alert-rules.yaml`](observability/prometheus-alert-rules.yaml)。规则文件
只拥有触发条件与稳定说明；Alertmanager receiver、通知渠道、静默、升级和最终 paging 分级属于
deployment policy，不得在通用仓库中写入团队地址或凭证。

### 6.3 告警

分页告警限于：多窗口 error-budget burn、evidence append 连续失败、Writer command log/CAS 基础设施不可用、大面积 readiness 失败、projection oldest-pending 持续超过 freshness SLO。
可用性与延迟优先使用 30 天目标的多窗口 burn-rate：快速通道使用 `1h+5m @ 14.4x` 和 `6h+30m @ 6x`，慢速工单使用 `1d+2h @ 3x`。长窗口和短窗口必须同时超标；低流量时必须有最小 eligible event 门槛，并由黑盒 canary/readiness 补位，不用一次失败计算出 100% error rate 就 page。

partial 比例、单 provider 延迟、投影重建频繁、拒绝率异常、磁盘容量和 telemetry drop 使用工单或趋势告警。单次 DENY、调用方输入错误和正常 CAS 冲突不得分页。

每条告警定义必须包含严重级、用户影响、稳定摘要、dashboard 和 runbook 链接、最小流量与解除条件。规则语法通过不等于告警有效；必须用可控错误、慢请求、队列堆积、Collector/backend 中断分别证明 firing 和 recovery。

### 6.4 身份与行为分析

身份分析分成三个不同用途，不得用一张“用户面板”混合权限：

| 用途 | 权威数据 | 规范聚合 | 不允许的做法 |
|---|---|---|---|
| 采用/产品 | access + system/audit + feedback evidence | DAU/WAU（user/agent/service 分开）、read/search/write 比例、Workspace/Repository 覆盖、零结果、partial、同 trace 内 refine、helpful 比例 | 从采样 trace 计精确 DAU；把零结果直接当错误 |
| 治理/审计 | access + system/audit evidence | 按实际 `principal`、`onBehalfOf`、action、decision、ruleId、固定 Repository/commit/object 的使用和拒绝 | 用 diagnostic log 替代不采样 evidence；丢掉 basis |
| 安全 | 可信认证事件 + access/audit evidence | deny burst、新/罕见 principal→Repository、写入爆发、异常代理比例、委托校验失败 | 把统计异常直接当入侵结论；向普通面板暴露原始身份 |

行为分析的精确事实从不采样 evidence 重放，不在请求路径中同步计算。高基数聚合使用受控分析存储/SIEM 或定时作业；Prometheus 只接收“每日活跃主体数”、“代理请求比例”这类已聚合且不带 principal 标签的有界结果。

`principal` 和 `onBehalfOf` 只有在认证边界已建立、委托已验证时才可用于安全结论。参考实现当前对字段只做形状校验的部署，只能做本地调试/采用分析，不得声称已有可信委托审计。

所有行为数据都必须有目的、访问控制、保留期、删除政策和身份摘要密钥轮换规则。不得记录 token、凭证、完整 query 或知识正文。

### 6.5 面板与诊断路径

面板按决策层级组织，不按 Prometheus/Jaeger/Loki 产品名组织：

1. **System Status**：关键旅程 SLO、error-budget remaining/burn、流量、发布/配置变更标记。
2. **Journey**：Resolve、READ、SEARCH、Writer、Projection/Binding、VFS 分面的 rate/error/duration/quality。
3. **Dependency & Capacity**：Snapshot/OpenSearch/resource runtime/hook/identity provider 的延迟与错误，并展示 fan-out、queue、in-flight 和宿主 USE。
4. **Telemetry Pipeline**：SDK drop、Collector accepted/refused/queue/send-failed、backend ingest/query/storage 和 canary。
5. **Identity & Behavior**：受 `audit`/安全权限保护的聚合采用、委托、拒绝和异常，不展示原始主体列表。
6. **Investigation**：从上述面板传入时间、operation/provider/outcome，再跳到 slow/error trace，由 traceId 关联 log 和授权查询的 evidence。

典型排障顺序固定为：

| 症状 | 先分 | 再定位 |
|---|---|---|
| SEARCH 慢 | complete/partial/error，总耗时与 plan/probe/hydrate/orchestration | candidate amplification → provider/snapshot child span → slow trace/log |
| SEARCH 不完整/结果旧 | partial reason、projection freshness/coverage | outbox oldest → transition/rebuild → target/basis 受控事件 |
| READ/Writer 失败 | caller result 与 technical error/conflict 分开 | Schema/ledger/CAS/snapshot 阶段 → authority CLIENT span |
| 大面积慢 | journey active/rate 与 CPU/GC/RSS/FD/network/disk | 先找饱和和 queue，再看最高耗时 operation/trace |
| 身份/拒绝异常 | authn 与 authz、principal kind、provider | 受权 evidence/SIEM 的 principal/onBehalfOf/ruleId/basis |
| “没数据” | app exporter drop 与 Collector ingress 先分界 | Collector queue/send → Jaeger/Loki/Prometheus backend 自身健康 → canary |

发布版本和配置 digest 必须作为低基数 Resource/build info 和 dashboard annotation 可见，使“最近什么变了”成为默认调查步骤。

---

## 7. 采样、安全与代码边界

### 7.1 采样与保留

- access、feedback、system/audit evidence 不采样，按各自访问控制和合规策略保留。
- metric 不采样；进程内聚合后导出。
- 需要“错误、拒绝、partial、写请求 100% 保留”的共享部署，入口必须 record 全部 span，并由 Collector tail sampling 全量保留这些 outcome、按比例保留普通成功读。已经被 SDK head sampling 丢弃的 span 不能在尾采样恢复。
- 资源受限的本地 profile 可以使用 parent-based ratio sampling，但不得宣称错误 trace 100% 保留。
- exporter queue、batch、timeout、retention 和采样率属于 deployment config，不得进入 Repository、Catalog 或知识 Schema。

### 7.2 身份与敏感数据

`principal`、`onBehalfOf` 不得进入 baggage 或 metric。access evidence 保留原始可信身份；普通 telemetry 默认只记录 `kc.principal.kind=owner|user|agent|service|other`。若部署需要跨 trace 关联主体，只能使用部署侧密钥产生、可轮换且限定作用域的摘要，并记录该策略版本；不得使用可逆编码或无盐散列。

### 7.3 代码边界

```text
cli / cmd/kc                 transport、propagation、OTel SDK/exporter 装配
        │
        ▼
internal/telemetry           进程级 OTel SDK/Prometheus/OTLP runtime、稳定词表
        │
        ├── catalog / snapshot / knowledge / index / retrieval
        └── hook / gate / controlplane / workspacefs

observability/               access / feedback / trace query / hitmap 证据
internal/journal             system / kc 本机过程账
```

1. `internal/telemetry` 可以封装 OpenTelemetry API/SDK 和通用 exporter，但不得依赖 KC 领域类型或具体 vendor backend；`cli`/`cmd` 只负责创建、配置和关闭 process runtime。
2. 新服务/应用入口必须传 `context.Context`；context 不进入协议对象、digest、Repository 文件或 Catalog 状态。
3. 不给 `snapshot.Store` 增加 metrics/log 方法，不给 Catalog 增加知识访问字段；I/O 明细使用 context-aware decorator 或 adapter 埋点。
4. `observability/` 保留版本化访问证据语义；generic telemetry 类型不得依赖 `knowledge`，避免底层包形成反向或循环依赖。
5. 默认实现必须是低开销 no-op，使离线库调用不要求运行 Collector。
6. 业务结果先确定，再依次完成必须的 evidence 和非阻塞 telemetry；evidence 按第 1 节影响交付，telemetry 永不覆盖业务结果。

操作名、属性词表、metric instrument 和 telemetry schema version 由一个位置维护。测试必须拒绝未知 label、自由文本 label、敏感字段和超出基数预算的 instrument。

---

## 8. 实现与运行证据

本文只拥有 metric/log/trace、健康和 SLI/SLO 合同，不维护参考实现功能清单或阶段 A–D
落地顺序。当前 endpoint、OTel runtime、采样、drop、evidence 关联与尚未覆盖的信号统一登记
在 `TEST_CATALOG.md` 和根 `README.md`；产品生产就绪判断统一登记在 `MVP_ACCEPTANCE.md`。

实现可以分阶段，但任何阶段都不能改变前七节的失败边界、低基数和敏感数据规则。

---

## 9. Conformance

1. 同一次 HTTP/CLI 消费能用 requestId/traceId 关联 completion log、system/audit journal 和固定版本 access event。
2. 非法或冲突 trace header 不使业务请求失败；合法 `traceparent` 胜出，传播 outcome 可见。
3. exporter/Collector 完全不可用时协议结果不变；本机 drop 累计和限速 stderr event 可见，恢复后 drop metric 可导出。
4. access evidence 不可写时，必须审计的成功消费不会交付成功；Evidence coverage 对账恒等于 100%。
5. 代码只创建 OTel instrument；Prometheus 名由 exporter 映射，同一信号没有双重 instrument。
6. metric attribute 中不存在 Repository、commit、object、Address、path、principal、requestId、traceId、pinId、commandId、evidenceId 或自由文本。
7. SEARCH dashboard 区分 unavailable、partial、complete，并显示 candidate、hydrate、drop 和 partial reason；provider 成功不等于完整知识结果。
8. projection 落后不使 Canonical READ 失败，但反映在 component health、oldest pending、freshness SLI 和 Search claims 中。
9. `/livez` 不因远端依赖失败而抖动；总 `/readyz` 等于所有启用 Surface readiness 的 AND，分面结果可诊断。
10. log/span event/metric 不包含知识正文、secret、token、完整查询文本或未脱敏外部响应。
11. head sampling profile 不宣称错误 trace 全量；需要全量错误时有 Collector tail-sampling 验收证据。
12. `go test ./internal/arch/` 继续证明 telemetry 没有改变 ⓪–③ 依赖方向。
13. Jaeger 的 System Architecture 只展示被分布式 span 实际观测到的服务依赖，不作为
    静态系统架构图；未具备 CLIENT/SERVER span 和跨进程传播的依赖必须明确显示为未覆盖。

---

## 参考标准

- [Google SRE: Monitoring Distributed Systems](https://sre.google/sre-book/monitoring-distributed-systems/)：症状/原因、黑盒/白盒、Golden Signals 与低噪声告警。
- [Google SRE Workbook: Implementing SLOs](https://sre.google/workbook/implementing-slos/)：good/valid event、request/pipeline/storage 的 availability、latency、quality、freshness、coverage 和 durability SLI。
- [Google SRE Workbook: Alerting on SLOs](https://sre.google/workbook/alerting-on-slos/)：error budget 与多窗口多 burn-rate 告警。
- [The USE Method](https://www.brendangregg.com/usemethod.html)：对每个有限资源检查 utilization、saturation 和 errors。
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)：`traceparent` / `tracestate` 传播与非法上下文处理。
- [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/)：HTTP span/metric、Resource、Log Data Model、metric naming/unit。
- [OpenTelemetry Collector Internal Telemetry](https://opentelemetry.io/docs/collector/internal-telemetry/)：Collector 的 ingress/egress、refused、enqueue/send failure 和 queue 自监控。
- [Prometheus Instrumentation](https://prometheus.io/docs/practices/instrumentation/) / [Metric and Label Naming](https://prometheus.io/docs/practices/naming/)：RED 原始信号、exposition name、base unit 与 label cardinality。
- [Prometheus Alerting](https://prometheus.io/docs/practices/alerting/)：优先用户症状、告警可执行与控制噪声。
- [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)：用户身份、动作、对象、结果和 interaction identifier 的安全事件设计。
- [Kubernetes Liveness/Readiness/Startup Probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/)：存活与流量接纳语义。
- [BCP 14（RFC 2119）](https://www.rfc-editor.org/rfc/rfc2119) / [RFC 8174](https://www.rfc-editor.org/rfc/rfc8174)：规范性关键词。

实现阶段可选使用 Apache-2.0 的
[`o11y-dev/opentelemetry-skill`](https://github.com/o11y-dev/opentelemetry-skill)
辅助检查 Collector、采样、基数、安全和管线自监控。它是工具化实现辅助，不是本项目 SLI、SLO、风险模型或分层的权威来源。
