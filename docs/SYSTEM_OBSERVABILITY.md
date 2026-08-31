# 系统可观测性规范

日期：2026-08-31

定位：metric/log/trace、健康与 SLI/SLO 的规范草案；实现状态只在
`MVP_ACCEPTANCE.md` / `TEST_CATALOG.md` 维护。

本文定义 Knowledge Catalog 运行系统怎样发现故障、解释性能、衡量可靠性，并把一次请求关联到已有的知识访问证据。它不改变 ⓪–③ 协议分层，也不把 metric、diagnostic log 或 distributed trace 写成知识。

术语以 [`TERMINOLOGY.md`](TERMINOLOGY.md) 为准；知识访问证据以 [`OBSERVABILITY.md`](OBSERVABILITY.md) 为准。本文中的“必须/不得”表示不可违反的合同，“应当/不应当”表示除非有明确记录的理由否则遵守，“可以”表示可选，含义与 BCP 14（RFC 2119/RFC 8174）一致。

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

instrument 专用 attribute 的稳定值域：

| 属性 | 允许值/来源 |
|---|---|
| `kc.authorization.decision` | `allow|deny` |
| `kc.writer.surface` | `COMMIT|PROPOSAL|other` |
| `kc.writer.replayed` | boolean |
| `kc.writer.change.operation` | `PUT|REMOVE|other` |
| `kc.projection.mode` | `incremental|rebuild|ready|other` |
| `kc.projection.cause` | `content|schema|ready|cold|diverged|other` |
| `kc.projection.from_state` / `kc.projection.to_state` | 与 `kc.projection.state` 相同 |
| `kc.hook.phase` | `pre|post|other` |
| `kc.hook.transport` | `exec|http|outbox|other` |
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
  "body": "kc.operation.completed",
  "trace_id": "...",
  "span_id": "...",
  "attributes": {
    "kc.request.id": "...",
    "kc.face": "knowledge",
    "kc.operation": "search",
    "kc.outcome": "partial",
    "kc.duration_ms": 84
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
| `kc.authorization.decisions` | Counter | `{decision}` | `kc_authorization_decisions_total` | `kc.operation`、`kc.authorization.decision` |
| `kc.workspace.resolve.duration` | Histogram | `s` | `kc_workspace_resolve_duration_seconds` | `kc.outcome` |
| `kc.workspace.member.count` | Histogram | `{repository}` | `kc_workspace_member_count` | `kc.outcome` |
| `kc.snapshot.operations` | Counter | `{operation}` | `kc_snapshot_operations_total` | `kc.snapshot.store`、`kc.operation`、`kc.outcome`、`error.type` |
| `kc.snapshot.operation.duration` | Histogram | `s` | `kc_snapshot_operation_duration_seconds` | `kc.snapshot.store`、`kc.operation`、`kc.outcome` |
| `kc.search.requests` | Counter | `{request}` | `kc_search_requests_total` | `kc.retrieval.provider`、`kc.search.completeness`、`kc.search.partial_reason`、`kc.outcome` |
| `kc.search.duration` | Histogram | `s` | `kc_search_duration_seconds` | `kc.retrieval.provider`、`kc.search.completeness`、`kc.outcome` |
| `kc.search.phase.duration` | Histogram | `s` | `kc_search_phase_duration_seconds` | `kc.retrieval.provider`、`kc.search.completeness`、`kc.outcome`、`kc.search.phase=plan|probe|hydrate|orchestration` |
| `kc.search.candidate.count` | Histogram | `{candidate}` | `kc_search_candidate_count` | `kc.retrieval.provider` |
| `kc.search.hydrated.count` | Histogram | `{object}` | `kc_search_hydrated_count` | `kc.retrieval.provider` |
| `kc.search.dropped.count` | Histogram | `{candidate}` | `kc_search_dropped_count` | `kc.retrieval.provider`、`kc.search.partial_reason` |
| `kc.writer.commands` | Counter | `{command}` | `kc_writer_commands_total` | `kc.writer.surface`、`kc.outcome`、`error.type`、`kc.writer.replayed` |
| `kc.writer.change.count` | Histogram | `{change}` | `kc_writer_change_count` | `kc.writer.surface`、`kc.writer.change.operation` |
| `kc.projection.transitions` | Counter | `{transition}` | `kc_projection_transitions_total` | `kc.retrieval.provider`、`kc.projection.from_state`、`kc.projection.to_state`、`kc.projection.cause` |
| `kc.projection.duration` | Histogram | `s` | `kc_projection_duration_seconds` | `kc.retrieval.provider`、`kc.projection.mode`、`kc.outcome` |
| `kc.projection.lagging.count` | ObservableGauge | `{projection}` | `kc_projection_lagging_count` | `kc.retrieval.provider` |
| `kc.projection.oldest_pending.age` | ObservableGauge | `s` | `kc_projection_oldest_pending_age_seconds` | `kc.retrieval.provider` |
| `kc.evidence.appends` | Counter | `{append}` | `kc_evidence_appends_total` | `kc.evidence.kind`、`kc.outcome` |
| `kc.evidence.append.duration` | Histogram | `s` | `kc_evidence_append_duration_seconds` | `kc.evidence.kind`、`kc.outcome` |
| `kc.telemetry.dropped` | Counter | `{record}` | `kc_telemetry_dropped_total` | `kc.telemetry.signal`、`kc.telemetry.drop_reason` |
| `kc.hook.dispatches` | Counter | `{dispatch}` | `kc_hook_dispatches_total` | `kc.hook.phase`、`kc.hook.transport`、`kc.outcome` |

`kc.operation` 的公开动词来自 `cli/command.go`，内部操作由 telemetry 词表显式登记。未登记值映射为 `other`。

参考实现提供覆盖当前 reference objective 的默认 Histogram bucket；deployment profile 可通过 OTel View 覆盖，但 Dolt 与远程 Gitea/OpenSearch 不应共用一组未经基线验证的阈值。instrument 的名称、类型、unit 或属性语义发生破坏性变化时，必须提升 telemetry schema version；稳定 dashboard 使用的旧 instrument 至少跨一个发布周期双发或提供 recording-rule 迁移。

### 4.2 Drop 的可观察性

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

### 6.2 规范聚合视图

应用只发 Counter、Histogram 和 Gauge 原始样本；P50/P95/P99、比率和窗口聚合由
Prometheus recording rules 派生。参考规则见
[`observability/prometheus-recording-rules.yaml`](observability/prometheus-recording-rules.yaml)，至少提供：

- 完整 SEARCH 的 P50/P95/P99，总耗时按 provider 分组；
- `plan|probe|hydrate|orchestration` 各阶段 P95；
- SEARCH/Writer availability、Evidence durability；
- partial ratio，按稳定 partial reason 分组；
- 每请求 candidates、hydrated 平均值，以及 candidate amplification、drop ratio。

完整 SEARCH 的主判断顺序是：先看 availability/completeness，再看总 P95/P99，然后用阶段
P95 和 candidate amplification 定位。只看平均耗时会掩盖长尾；只看总耗时无法区分 provider
查询慢、Canonical hydrate 慢或 Workspace 编排放大。

### 6.3 告警

分页告警限于：多窗口 error-budget burn、evidence append 连续失败、Writer command log/CAS 基础设施不可用、大面积 readiness 失败、projection oldest-pending 持续超过 freshness SLO。

partial 比例、单 provider 延迟、投影重建频繁、拒绝率异常、磁盘容量和 telemetry drop 使用工单或趋势告警。单次 DENY、调用方输入错误和正常 CAS 冲突不得分页。

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

---

## 参考标准

- [W3C Trace Context](https://www.w3.org/TR/trace-context/)：`traceparent` / `tracestate` 传播与非法上下文处理。
- [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/)：HTTP span/metric、Resource、Log Data Model、metric naming/unit。
- [Prometheus Metric and Label Naming](https://prometheus.io/docs/practices/naming/)：exposition name、base unit 与 label cardinality。
- [Kubernetes Liveness/Readiness/Startup Probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/)：存活与流量接纳语义。
- [BCP 14（RFC 2119）](https://www.rfc-editor.org/rfc/rfc2119) / [RFC 8174](https://www.rfc-editor.org/rfc/rfc8174)：规范性关键词。
