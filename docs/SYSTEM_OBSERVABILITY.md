# 系统可观测性设计

日期：2026-08-26

状态：目标设计

本文定义 Knowledge Catalog 运行系统怎样发现故障、解释性能、衡量可靠性，并把一次请求关联到已有的知识访问证据。它不改变 ⓪–③ 协议分层，也不把指标、日志或 trace 写成知识。

知识访问的审计契约仍以 [`OBSERVABILITY.md`](OBSERVABILITY.md) 为准。本文只补齐服务与组件的运行可观测性。

---

## 1. 结论

系统保留四类用途不同的数据，不能混成一条“日志”：

| 数据 | 回答的问题 | 可靠性 | 权威性 |
|---|---|---|---|
| Canonical 与 git 历史 | 知识和治理状态是什么、怎样变化 | 由 Snapshot/Registry 保证 | 协议权威 |
| access / feedback / system / audit 证据 | 谁以哪个固定版本做了什么 | 不采样；按既有动作语义持久化 | 非 Canonical 的审计证据 |
| metric / diagnostic log / distributed trace | 系统是否健康、慢在哪里、为什么失败 | 可采样；异步导出；允许受控丢失 | 诊断遥测，不作授权或审计依据 |
| dashboard / alert / SLO report | 当前是否需要人介入 | 可从遥测重建 | 派生视图 |

最重要的失败边界：

- 必须记访问账的成功消费若无法持久化 access evidence，仍按现有语义返回失败；不能声称“成功且可审计”。
- OTLP、Prometheus 或诊断日志导出失败不得改变 READ、SEARCH、COMMIT、Catalog 操作的协议结果；只增加内部丢弃计数和降级告警。
- telemetry 不进入 Repository、Catalog pin、provenance、索引排序或权限判断。

---

## 2. 观察模型

一次请求的关联主键如下：

```text
requestId   一次传输/命令调用
traceId     一条跨服务调用链
spanId      调用链中的一个操作
sessionId   Agent 或交互会话，可选
pinId       一次消费任务冻结的 ResolvedWorkspace 标识
commandId   一次可重放写命令
```

这些 id 用途不同，不能互相代替。`pinId` 和 `commandId` 是业务一致性坐标；`requestId` 和 trace/span 是调用关联坐标。

服务入口优先接受 W3C `traceparent` / `tracestate`。现有 `X-Kc-Trace-Id`、`X-Kc-Span-Id`、`X-Kc-Parent-Span-Id` 作为兼容入口：仅在没有 `traceparent` 时使用；两套入口同时出现且不一致时拒绝，避免一条请求形成两棵 trace。缺少 `requestId` 时由入口生成，并在响应头和错误信封的诊断元数据中返回。

`principal`、`onBehalfOf` 继续进入受控的审计证据，但不放入 baggage，不作为 metric label。诊断遥测默认只保留主体类别（owner/user/agent/service）或部署侧盐化摘要，避免把身份扩散到普通监控系统。

---

## 3. 信号设计

### 3.1 Trace

顶层 span 覆盖完整请求，子 span 只围绕有独立失败或延迟价值的边界：

```text
kc.http.server / kc.cli.invoke
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

规则：

- 不为每个 search hit、Aspect member 或 tree entry 建 span；在父 span 上记数量和一次聚合事件。
- span 名只含稳定操作名，不拼 Repository、object、path、commit 或错误消息。
- `kernel` 错误记录枚举型 `kc.error.code`；原始错误文本只进入受限日志或 span event。
- `FORBIDDEN`、`NON_FAST_FORWARD` 等预期业务结果仍记录 outcome，但不能与进程崩溃、I/O 超时混为一种错误率。
- 一次 SEARCH 必须记录 completeness、provider、候选数、回读数、丢弃数和 continuation 是否产生；不能只看到索引查询成功。
- 一次 Workspace 消费必须记录同一个 `pinId`，不能在子 span 中重新跟随 selector。

建议保留的低基数字段：

```text
kc.face                 catalog | knowledge | writer | projection | vfs
kc.operation            稳定动词/内部操作
kc.outcome              ok | denied | conflict | partial | error
kc.error.code           kernel 错误码
kc.store.kind           filegit | gitea | dolt
kc.provider             sqlite | opensearch | starrocks
kc.search.completeness  complete | partial
kc.projection.state     BUILDING | READY | UPDATING | FAILED | RETIRED
```

Repository、commit、object、Address、path、principal、requestId、traceId、commandId 都是高基数，只进入 trace、审计证据或受限诊断日志，不进入 metric label。

### 3.2 Metrics

指标使用 Prometheus/OpenTelemetry 单调计数器、直方图和 observable gauge。首批稳定指标：

| 指标 | 类型 | 允许的 label |
|---|---|---|
| `kc_requests_total` | Counter | face、operation、outcome、error_code |
| `kc_request_duration_seconds` | Histogram | face、operation、outcome |
| `kc_requests_in_flight` | Gauge | face |
| `kc_authorization_decisions_total` | Counter | action、decision |
| `kc_workspace_resolve_duration_seconds` | Histogram | outcome |
| `kc_workspace_members` | Histogram | outcome |
| `kc_snapshot_operations_total` | Counter | store_kind、operation、outcome、error_code |
| `kc_snapshot_operation_duration_seconds` | Histogram | store_kind、operation、outcome |
| `kc_search_requests_total` | Counter | provider、completeness、outcome |
| `kc_search_duration_seconds` | Histogram | provider、completeness |
| `kc_search_candidates` / `kc_search_hydrated` / `kc_search_dropped` | Histogram | provider |
| `kc_writer_commands_total` | Counter | surface、outcome、error_code、replayed |
| `kc_writer_changes` | Histogram | surface、operation |
| `kc_projection_transitions_total` | Counter | provider、from_state、to_state、cause |
| `kc_projection_duration_seconds` | Histogram | provider、mode、outcome |
| `kc_projection_lagging` | Gauge | provider |
| `kc_evidence_append_total` | Counter | evidence_kind、outcome |
| `kc_evidence_append_duration_seconds` | Histogram | evidence_kind、outcome |
| `kc_telemetry_dropped_total` | Counter | signal、reason |
| `kc_hook_dispatch_total` | Counter | phase、transport、outcome |

`operation`、`action`、`surface`、`cause` 必须来自代码中的有限词表，不能直接使用用户输入。若运营需要按 Repository 排障，通过 trace/log 查询或受控的 Top-N 派生报表完成，不把 Repository id 加到通用时序标签。

Histogram bucket 由部署 profile 配置；本机 FileGit 与远程 Gitea/Elasticsearch 不共用一组武断的延迟阈值。

### 3.3 Diagnostic logs

诊断日志统一为结构化 JSON，一次边界调用正常情况下只写一条 completion event；内部只在重试、状态迁移、降级和异常时追加事件。最小字段：

```json
{
  "time": "...",
  "severity": "INFO",
  "service": {"name": "kc-server", "version": "...", "instanceId": "..."},
  "traceId": "...",
  "spanId": "...",
  "requestId": "...",
  "face": "knowledge",
  "operation": "search",
  "outcome": "partial",
  "durationMs": 84,
  "error": {"code": "TEMPORARY_UNAVAILABLE"}
}
```

默认不得写入：知识正文、ChangeSet value、Authorization/Cookie/token、Binding secret、query 原文、完整 argv、未脱敏外部响应。查询排障只记录规范化 query shape、clause 数和可选盐化摘要。

现有 `.kc/system.jsonl` 与 `.kc/audit.jsonl` 继续是 pointers-only 的本机过程账，不因接入普通 stdout/OTLP 日志而改名或降级；二者可共享 trace/request 关联字段，但存储和 retention 独立。

---

## 4. 健康与组件状态

服务提供三个不同视图：

| 入口 | 含义 | 检查范围 |
|---|---|---|
| `/livez` | 进程能否继续接请求 | 进程、事件循环；不探远端依赖 |
| `/readyz` | 当前实例能否安全承接基础流量 | 配置、Catalog/Home 控制状态、必须持久化的 evidence/journal 路径、必要本机状态 |
| `/health` | 兼容摘要 | 汇总 live/ready，不作为深度依赖探针 |

`GET /v1/_state` 是受权的产品状态，不是 Kubernetes 健康探针。`/metrics` 放在独立 management listener 或受网络/管理员权限保护的路径，不进入 KC 动词表。

远程 Repository、每个检索 provider 和每个 projection 的状态是 route/component health：

- 某个 Gitea Repository 不可用，不应让不相关 Repository 的整个实例 unready。
- projection 落后或失败不等于 Canonical READ 不可用；SEARCH 返回真实 completeness/claims，并单独暴露 projection 状态。
- access evidence 存储不可写会破坏成功消费的审计承诺，因此应使消费面 readiness 失败。
- 只做昂贵深探针会制造探针流量和级联故障；依赖状态优先来自真实请求和有退避的后台检查。

组件状态至少包含 `state`、`lastSuccessAt`、`lastErrorCode`、`consecutiveFailures`；不得在公开健康响应中暴露 home 路径、凭证、内部 endpoint 或原始错误正文。

---

## 5. SLI、SLO 与告警

可靠性按 surface 衡量，不能用一个“KC 可用率”掩盖部分故障。

| Surface | SLI | 说明 |
|---|---|---|
| Canonical READ | 合格请求中返回有效知识/明确未解析结果的比例 | 排除 USAGE_INVALID、UNAUTHENTICATED、FORBIDDEN 和调用方前置条件错误 |
| SEARCH availability | 返回合法 SearchResult 的比例 | `partial` 仍可用，但另计完整性 |
| SEARCH completeness | `complete` / 全部成功 SEARCH | 必须按 provider capability 和授权裁剪解释 |
| Writer availability | 合格写请求得到 commit/receipt 的比例 | NON_FAST_FORWARD、IDEMPOTENCY_CONFLICT 单独计冲突率 |
| Evidence coverage | 必须审计的成功动作拥有持久 access event 的比例 | 目标恒为 100%，不可采样 |
| Projection freshness | READY 且 basis 追上目标 head 的时间比例 | 同时观察最后成功更新时间，不能把索引延迟解释为知识不存在 |
| Latency | 各 surface 的 p50/p95/p99 | 按 store/provider/deployment profile 分开看 |

首个共享部署可用以下值启动容量和告警校准，但它们属于部署策略，不写进协议：Canonical READ 30 天可用率 99.9%，Writer 和 SEARCH availability 99.5%，受支持完整检索的 completeness 99%，projection 99% 在 5 分钟内追上目标 head，Evidence coverage 100%。延迟目标在真实负载基线后确定。

分页告警只覆盖需要立即处理的故障：

- 多窗口 error-budget burn；
- evidence append 连续失败或路径不可写；
- Writer command log/CAS 基础设施不可用；
- 大面积 readiness 失败；
- projection outbox 最老事件持续超过 freshness SLO。

工单或趋势告警覆盖 partial 比例升高、单 provider 延迟、投影重建频繁、拒绝率异常、磁盘容量和 telemetry drop。单次 DENY、调用方输入错误和正常 CAS 冲突不分页。

---

## 6. 采样、背压与保留

- access、feedback、system/audit evidence 不采样；按各自合规策略保留。
- 写请求、权限拒绝、partial SEARCH、错误 trace 100% 保留；普通成功读采用 parent-based ratio sampling，起始可为 5%。
- metric 不采样；本机先聚合再导出。
- telemetry exporter 使用有界队列和批量异步发送。队列满时优先丢普通成功 trace/debug log，并增加 `kc_telemetry_dropped_total`；不得阻塞协议路径。
- 错误和安全事件可用 tail sampling 提高保留率，但不能把它当 access evidence 的替代品。
- metric、trace、诊断日志和审计证据分别配置 retention。删除诊断遥测不影响 Canonical、访问账或 hitmap 的协议含义。

建议通过 OpenTelemetry SDK/Collector 输出 OTLP，Collector 再路由到 Prometheus 兼容指标、trace backend 和日志后端。核心包不认识具体 vendor，也不保存 exporter endpoint 或 secret。

---

## 7. 代码边界

运行遥测是横切装配，不是新的知识层：

```text
cli / cmd/kc                 transport、propagation、SDK/exporter 装配
        │
        ▼
internal/telemetry           context、稳定操作/字段词表、no-op 接口
        │
        ├── catalog / snapshot / knowledge / index / retrieval
        └── hook / gate / controlplane / workspacefs

observability/               access / feedback / trace query / hitmap 证据
internal/journal             system / kc 本机过程账
```

约束：

1. 核心包只依赖无 vendor 的 telemetry 接缝；OTel SDK、exporter、Prometheus handler 只在应用装配处出现。
2. 新的服务/应用入口传 `context.Context`；context 不进入协议对象、digest、Repository 文件或 Catalog 状态。
3. 不给 `snapshot.Store` 增加 metrics/log 方法，不给 Catalog 增加知识访问字段。需要 I/O 明细时用 context-aware decorator 或 adapter 埋点。
4. `observability/` 保留版本化访问证据语义；generic metric/span 类型不得反向依赖 `knowledge`，避免底层包经可观测性形成循环依赖。
5. 默认实现必须是低开销 no-op，使离线库调用不要求运行 Collector。
6. 业务返回值先确定，再分别完成 evidence 和 diagnostic telemetry：evidence 按其可靠性规则影响结果，telemetry 永不覆盖原始结果。

操作名和 attribute 词表由一个位置维护，并由测试拒绝未知的 metric label 值及敏感字段。不要让每个 adapter 自由发明近义指标。

---

## 8. 落地顺序

### 阶段 A：入口与关联

- 在 CLI/HTTP 统一入口生成或接收 request/trace 上下文；响应返回 request id。
- 给 `Invoke` 总耗时、outcome、kernel error code 和 evidence append 建首批指标/trace。
- 保持现有 access/journal 行为不变，验证同一 request/trace 能关联三类记录。

### 阶段 B：关键路径

- 埋点 Workspace resolve、Snapshot I/O、Reader、Writer CAS、SEARCH probe/candidate/hydrate。
- SEARCH 记录 completeness/claims 分类和 candidate→hydrate 漏斗。
- projection 记录状态迁移、basis/head 是否落后、重建原因和耗时。

### 阶段 C：运行出口

- 增加 `/livez`、`/readyz` 和受保护的 `/metrics`；保留 `/health` 兼容。
- 在应用装配处接 OTel SDK/Collector，提供本地 no-op 与开发 console profile。
- 建 Canonical read、Writer、SEARCH、projection、evidence 五张基础 dashboard。

### 阶段 D：SLO 闭环

- 用生产基线校准 histogram bucket、采样率和延迟 SLO。
- 配置多窗口 burn-rate、evidence durability、projection freshness 告警。
- 做 exporter 中断、evidence 路径只读、远端 Store 超时、projection 失败和高并发的故障演练。

---

## 9. 验收条件

1. 同一次 HTTP/CLI 消费可用 requestId/traceId 关联 completion log、system/audit journal 和固定版本 access event。
2. exporter/Collector 完全不可用时，协议结果不变，且 drop 指标可见；access evidence 不可写时，必须审计的成功消费不会被返回为成功。
3. metric label 中不存在 Repository、commit、object、Address、path、principal、requestId、traceId、commandId 或自由文本。
4. SEARCH dashboard 能区分不可用、partial、complete，并看到 candidate、hydrate、drop 数；不能把 provider 成功等同于完整知识结果。
5. projection 落后不会使 Canonical READ 失败，但会反映在 component health、freshness SLI 和 Search claims 中。
6. `/livez` 不因远端依赖失败而抖动；`/readyz` 能发现本机必需状态或 evidence 存储不可用。
7. 任何日志、span event、metric 都不包含知识正文、secret、token、完整查询文本或未脱敏外部响应。
8. `go test ./internal/arch/` 继续证明 telemetry 没有改变 ⓪–③ 的依赖方向。
