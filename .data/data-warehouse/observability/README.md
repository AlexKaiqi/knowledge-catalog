# 本地可观测性定义与边界

这个目录维护的是可重复部署的**定义**，不是运行时采集到的数据。`make dw-obs-up`
把同一套定义加载到本地 Compose 环境，`make dw-obs-smoke` 用真实 SEARCH 验证整条链路。

## 数据、定义与界面

| 类别 | 组件 | 本项目维护什么 | 运行时保存什么 |
|---|---|---|---|
| Metrics | Prometheus | scrape config、recording rules、dashboard PromQL | 24h 时序样本，Compose volume；不提交 |
| Traces | Jaeger | Collector export、Grafana datasource/link、smoke oracle | 进程内 trace；容器删除即消失 |
| Logs | Loki | 单机配置、OTLP pipeline、LogQL dashboard、字段合同 | `/loki` tmpfs 中最多 24h 日志；容器删除即消失 |
| 展示 | Grafana | datasource provisioning、四个 dashboard JSON | 本地 UI 状态不持久化；重启后从定义恢复 |
| 传输 | OTel Collector | OTLP receiver、trace/log pipelines | 只缓冲转发，不作为长期存储 |

因此 Git 中应评审的是 `*.yaml`、dashboard JSON、recording rules、日志事件合同和 smoke；
Prometheus 样本、Jaeger spans、Loki entries、Grafana UI 数据库都不是项目资产。

## 当前链路

```text
kc-server /metrics ───────────────▶ Prometheus ──┐
kc-server OTLP traces ─▶ OTel Collector ─▶ Jaeger├─▶ Grafana
kc-server OTLP logs   ─▶ OTel Collector ─▶ Loki  ┘
```

日志只给 `service_namespace` / `service_name` 两个低基数 Resource 属性建立 Loki 索引；
request id、route、status、duration、trace id 和 span id 是 structured metadata。不得把
request id、object、Repository、principal 或 query 做 Loki label。

Jaeger 的 **System Architecture** 是根据分布式 span 推导的运行时服务依赖图，不是项目静态
架构图。目前只有 `kc-server` SERVER/application spans，所以该页只会看到单服务内部关系。
Gitea、OpenSearch、resource-access 和 MySQL 尚未全部具备成对 CLIENT/SERVER span 与上下文
传播；在补齐前，项目面板只链接 `kc.search` trace，不把 Jaeger 依赖页称作系统架构证明。

## 已知不足（按优先级）

| 优先级 | 不足 | 当前处理 |
|---|---|---|
| P0 | 原始 SEARCH 总耗时、阶段、候选/hydrate/drop 无法聚合理解 | 已有原始 histogram、12 条 recording rules 和 SEARCH dashboard |
| P0 | 没有日志消费后端与预定义视图 | 已接 Loki、OTLP logs、Logs dashboard，并在 smoke 中与 Jaeger 对同一 traceId |
| P1 | 健康探针制造大量无价值 trace/log | 已抑制；仍保留 HTTP metric |
| P1 | OpenTelemetry Go Logs SDK 仍处于 beta | 事件合同独立在 `internal/telemetry/log_contract.go`，依赖版本固定；升级必须重跑字段与 OTLP smoke |
| P1 | 下游跨服务 trace 不完整，Jaeger 依赖图会误导 | 尚未完成；必须逐个 adapter 加 context-aware CLIENT span 与传播，不能虚构 service |
| P1 | 告警定义与生产通知容易混为一谈 | 已有 7 条版本化 Prometheus reference alerts；Alertmanager 路由、值班分级和 paging policy 仍由部署方维护 |
| P2 | 本地 trace/log 不持久 | 有意如此；生产部署须另定 retention、对象存储、备份和租户隔离 |
| P2 | 尚无规模基线、error-budget burn 与故障演练 | 本地 smoke 只证明链路和字段正确，不证明生产 SLO |

## 入口

- 系统总览：`http://127.0.0.1:7300/d/kc-overview/knowledge-catalog-system-overview`
- SEARCH 分析：`http://127.0.0.1:7300/d/kc-search-analysis/knowledge-catalog-search-analysis`
- 运行时健康：`http://127.0.0.1:7300/d/kc-runtime-health/knowledge-catalog-runtime-health`
- 诊断日志：`http://127.0.0.1:7300/d/kc-logs/knowledge-catalog-diagnostic-logs`
- Prometheus：`http://127.0.0.1:9090`
- Jaeger：`http://127.0.0.1:16686`
- Loki API：`http://127.0.0.1:3100`

```bash
make dw-obs-up
make dw-obs-smoke
make dw-obs-down
```
