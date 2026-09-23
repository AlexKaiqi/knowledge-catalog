# internal/telemetry

应用运行遥测 adapter；不拥有知识协议或访问证据。目标、信号取舍、敏感性与 SLI/SLO 由 [`docs/SYSTEM_OBSERVABILITY.md`](../../docs/SYSTEM_OBSERVABILITY.md) 拥有。下列选定名称和枚举集中在本包维护，instrument 登记见 `instruments.go`，bucket 见 `metric_contract.go`，行为与 normalization 见 `runtime.go`。清单保留设计要求的信号，不代表每项已经实现；覆盖以 [`docs/reviewed/test-catalog.md`](../../docs/reviewed/test-catalog.md) 为准。

## 属性与结果词表

所有跨 Signal 的通用 attribute 必须来自包内稳定词表；未知枚举统一映射为 `other`，不得把原始值降级成 label。单个 instrument 的专用 attribute 还必须满足下文 instrument 清单的值域。

| 属性 | 允许值/来源 | Signal |
|---|---|---|
| `kc.face` | `catalog\|knowledge\|writer\|projection\|vfs\|control\|hook\|gate\|other` | metric、span、log |
| `kc.operation` | CLI 命令表或内部稳定操作表 | metric、span、log |
| `kc.outcome` | `ok\|partial\|unresolved\|denied\|invalid\|conflict\|error` | metric、span、log |
| `error.type` | 失败时的稳定 kernel code；未知技术错误为 `other` | metric、span、log |
| `kc.snapshot.store` | `lakefs\|gitea\|other` | metric、span、log |
| `kc.retrieval.provider` | `none\|opensearch\|other` | metric、span、log |
| `kc.search.completeness` | `complete\|partial` | metric、span、log |
| `kc.search.partial_reason` | `authorization\|unsupported\|projection\|hydrate\|binding\|other` | metric、span、log |
| `kc.projection.state` | `BUILDING\|READY\|UPDATING\|FAILED\|RETIRED\|other` | metric、span、log |
| `kc.propagation.outcome` | `accepted\|generated\|legacy\|invalid\|conflict` | metric、span、log |
| `kc.principal.kind` | `owner\|user\|agent\|service\|other` | metric、span、log |
| `kc.identity.provider` | `local\|gitea\|oidc\|taihu\|other` | metric、span、log |
| `kc.binding.mode` | `state\|stream\|other` | metric、span、log |

instrument 专用 attribute 的稳定值域：

| 属性 | 允许值/来源 |
|---|---|
| `kc.authorization.decision` | `allow\|deny` |
| `kc.identity.delegated` | boolean；只表示已验证身份是否包含委托，不携带主体值 |
| `kc.writer.surface` | `COMMIT\|PROPOSAL\|other` |
| `kc.writer.replayed` | boolean |
| `kc.writer.change.operation` | `PUT\|REMOVE\|other` |
| `kc.projection.mode` | `incremental\|rebuild\|ready\|other` |
| `kc.projection.cause` | `content\|schema\|ready\|cold\|diverged\|other` |
| `kc.projection.change.operation` | `update\|remove` |
| `kc.projection.from_state` / `kc.projection.to_state` | 与 `kc.projection.state` 相同 |
| `kc.hook.phase` | `pre\|post\|other` |
| `kc.hook.transport` | `exec\|http\|outbox\|other` |
| `kc.outbox.kind` | `projection\|hook\|other` |
| `kc.gate.required` | boolean；当前 merge 是否声明至少一项 gate requirement |
| `kc.evidence.kind` | `access\|retrieval\|refine\|feedback\|system\|audit\|other` |
| `kc.telemetry.signal` | `metric\|log\|trace\|other` |
| `kc.telemetry.drop_reason` | `queue_full\|timeout\|export_error\|shutdown\|other` |

映射规则：

- 成功且完整 → `ok`；合法 partial SearchResult → `partial`。
- `*_UNRESOLVED` → `unresolved`；`UNAUTHENTICATED` / `FORBIDDEN` → `denied`。
- `USAGE_INVALID` / 调用方 `PRECONDITION_FAILED` → `invalid`。
- `NON_FAST_FORWARD` / `IDEMPOTENCY_CONFLICT` → `conflict`。
- `TEMPORARY_UNAVAILABLE`、I/O、超时、内部不变量失败 → `error`。
- `error.type` 只在非 `ok`/`partial` outcome 上出现；不得用错误消息作属性值。


## Instrument 合同

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
| `kc.read.object.count` | Histogram | `{object}` | `kc_read_object_count` | 无 |
| `kc.read.unit.count` | Histogram | `{unit}` | `kc_read_unit_count` | 无 |
| `kc.search.requests` | Counter | `{request}` | `kc_search_requests_total` | `kc.retrieval.provider`、`kc.search.completeness`、`kc.search.partial_reason`、`kc.outcome` |
| `kc.search.duration` | Histogram | `s` | `kc_search_duration_seconds` | `kc.retrieval.provider`、`kc.search.completeness`、`kc.outcome` |
| `kc.search.phase.duration` | Histogram | `s` | `kc_search_phase_duration_seconds` | `kc.retrieval.provider`、`kc.search.completeness`、`kc.outcome`、`kc.search.phase=plan\|probe\|hydrate\|orchestration` |
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
| `kc.projection.change.count` | Histogram | `{document}` | `kc_projection_change_count` | `kc.retrieval.provider`、`kc.projection.change.operation=update\|remove` |
| `kc.binding.lookups` | Counter | `{lookup}` | `kc_binding_lookups_total` | `kc.binding.mode`、`kc.outcome`、`error.type` |
| `kc.binding.lookup.duration` | Histogram | `s` | `kc_binding_lookup_duration_seconds` | `kc.binding.mode`、`kc.outcome` |
| `kc.binding.observation.age` | Histogram | `s` | `kc_binding_observation_age_seconds` | `kc.binding.mode`、`kc.outcome` |
| `kc.evidence.appends` | Counter | `{append}` | `kc_evidence_appends_total` | `kc.evidence.kind`、`kc.outcome` |
| `kc.evidence.append.duration` | Histogram | `s` | `kc_evidence_append_duration_seconds` | `kc.evidence.kind`、`kc.outcome` |
| `kc.evidence.append.bytes` | Histogram | `By` | `kc_evidence_append_bytes` | `kc.evidence.kind`、`kc.outcome` |
| `kc.evidence.store.used_ratio` | ObservableGauge | `1` | `kc_evidence_store_used_ratio` | 无 |
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
Snapshot 内部操作限于 `resolve_ref\|read\|read_many\|list_page\|history\|diff\|commit\|compare_and_swap\|other`；不得将 ref、path 或 Repository 拼入操作名。


OTel 名称是代码/OTLP 名称，Prometheus 名称是 exporter 映射；不得重复注册两套 instrument。属性语义的破坏性变化遵守系统可观测性 owner 的 schema-version 和迁移要求。

## Diagnostic log 形状

诊断日志使用 OTel Log Data Model；事件字段以 `log_contract.go` 和 `cli/serve_telemetry.go` 为准。以下是边界 completion log 的形状示例：

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

Resource 必须设置 `service.namespace=knowledge-catalog`、`service.name`、`service.version`、全局唯一的 `service.instance.id` 和 `kc.telemetry.schema.version`。

## Recording / alert 规则验证

规则和回归输入在 [`docs/observability/`](../../docs/observability/)。Canonical READ 的 `kc.operation` 是 CLI/HTTP 命令 `knowledge-read`，不是 `read`。Agent 读 [`agent-signals.json`](../../docs/observability/agent-signals.json)；Grafana 是同一套规则的人机投影。`make check-observability` 校验规则语法、查询包与 dashboard 选择器，并运行 [`prometheus-recording-rules-tests.yaml`](../../docs/observability/prometheus-recording-rules-tests.yaml)，结果进入 `.validation/runs/`。本地优先使用已有 `promtool`，否则只运行缓存的 `prom/prometheus:v3.5.0` 工具镜像；禁用拉取和网络，规则目录只读，不启动监控服务。CI 在独立步骤准备该固定版本，并始终归档验证证据。

回归覆盖成功序列尚未出现时的全失败窗口、全成功、空序列、零事件、只有排除流量和分组隔离，检查 SEARCH / READ / Writer / evidence / Binding 的 availability，以及多窗口 error ratio、burn、30 天预算和三类 FastBurn 告警。成功分子缺失时只由同组的合格总流量补零；分母必须为正，不能用全局 `vector(0)` 把无流量伪造为成功或失败。PromQL 向量匹配语义见 [Prometheus operators](https://prometheus.io/docs/prometheus/latest/querying/operators/)。
