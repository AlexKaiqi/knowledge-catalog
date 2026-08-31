# 压测环境要求与当前就绪性审计

审计日期：2026-08-31

结论：**当前环境不具备执行 `KC-PERF-01`–`KC-PERF-12` 并产出有效容量结论的条件。**
现有 Compose 拓扑和观测链可用于功能验收、S0 数据生成 smoke 与小规模本地探针；S1
以上仍被 load runner、真实历史生成、Dolt 服务形态、独立发压端、容量和证据留存阻断。

本文检查的是“环境能否支撑压测结论”，不是服务是否能启动。用例定义见
[`CASES.md`](CASES.md)，规模档位和资格门槛见
[`docs/SCALE_BENCHMARK.md`](../../../docs/SCALE_BENCHMARK.md)。

## 1. 就绪判定分层

| 层级 | 必须回答的问题 | 当前结论 |
| --- | --- | --- |
| 功能拓扑 | KC、MySQL、Dolt、OpenSearch 和观测服务能否连通 | `READY`，仅限本地功能用途 |
| 压测工具链 | 能否按 case、速率、并发和阶段发压并生成完整报告 | `BLOCKED` |
| S1/nightly | 能否稳定运行小规模性能回归并与批准基线比较 | `BLOCKED` |
| S2/pilot | 是否满足 pilot 参考资源和三节点 OpenSearch | `BLOCKED` |
| S3/release candidate | 是否有隔离容量环境、持久介质和恢复证据 | `BLOCKED` |
| S5/H4 qualification | 是否具备过亿对象、2,000 万真实 commit 的资格环境 | `BLOCKED` |

`READY` 只表示该层所有硬前置都具备；`PARTIAL` 不能执行正式用例；`BLOCKED` 表示即使
开始发流量，结果也只能作为调试信息，不能写成性能基线或资格结论。

## 2. 各档环境要求

### 2.1 所有压测共用要求

- 被测 revision、工作树 patch digest、Go/Dolt/OpenSearch exact version 或 image digest；
- 隔离的 Repository、Projection generation、端口、volume 和证据目录；
- load generator 与被测服务分进程；正式 S2 以上需分机或至少分容器并限制资源；
- 发压端 CPU 持续低于 70%，时钟同步且记录 generator→server RTT；
- 固定数据 seed、实际对象/unit/endpoint/eligible-doc 数、canonical bytes 和 digest；
- 可配置 open/closed-loop 到达、目标 RPS、并发、持续时间、warm-up/cool-down；
- Prometheus 原始 histogram、系统资源、Dolt SQL/query scan、OpenSearch node/index、trace、
  timeline、capacity 和 correctness 证据可覆盖整个测量及 cool-down 窗口；
- 证据保留时间必须长于最长用例，报告生成后仍可复算；
- 磁盘使用超过 85%、发压端饱和、采样缺口或配置漂移时自动判 `INVALID`。

### 2.2 S0 本地探针

S0 只用于校验 generator、runner、指标和报告链，不用于推断 S2–S5。允许同机部署，
但仍必须有独立 load-generator 进程、固定资源上限和可复算 `report.json`。

### 2.3 S2 pilot

沿用总体设计的最低参考档：

- Dolt：16 physical vCPU、64 GiB RAM、本地 NVMe，容量至少为预测 target bytes 的 3 倍；
- Dolt 以独立 `dolt sql-server` 长连接服务运行，使用连接池和单 active-writer lease；
- Writer 与 load generator 分机或分容器；
- OpenSearch 至少 3 个 data nodes，每 node 16 vCPU、64 GiB RAM；
- shard、replica、refresh、heap、durability 和网络 RTT 固定并写入 manifest。

### 2.4 S5/H4 qualification

建议从 Dolt 64 vCPU、256 GiB RAM 和企业 NVMe 起步；最终容量根据 S2/S3 实测
bytes/object、bytes/commit、primary bytes/doc、备份和 rebuild 临时空间计算。OpenSearch
节点数不能写死，但必须满足磁盘利用率不超过 60%，并容纳新旧 generation 同时存在。
H4 必须是 2,000 万次真实 row changes，不能用空 commit 或只在 manifest 中写目标数。

## 3. 2026-08-31 当前环境实测

### 3.1 主机与 Docker

| 项目 | 实测 | 判定 |
| --- | --- | --- |
| 主机 | macOS 15.7.4，arm64，12 physical/logical CPU，64 GiB RAM | S0 可用；低于 S2 的 16 physical vCPU 参考档 |
| 主机磁盘 | 926 GiB 总量，约 507 GiB 可用 | 能做本地探针；因尚无实测 target bytes，无法证明 3 倍容量 |
| Docker | Desktop 4.57.0，Engine 29.1.3，12 CPU | daemon 正常 |
| Docker 内存 | 8,217,165,824 bytes，容器视角约 7.65 GiB | 不满足 S2；OpenSearch、MySQL、KC 共用该上限 |
| Docker 资源隔离 | KC/MySQL/OpenSearch 均未设置 CPU、memory、PID limit | 无法形成可复现性能基线 |
| 工作树 | revision `1c6308f4cae53cba64fb72a51ad4b0d7105680d0`，存在未提交修改 | 未保存 patch digest 前不能作为可复现基线 |

Docker 虚拟盘报告的可用空间不能替代宿主物理可用空间；容量判断取二者较小值，并还需
扣除 rebuild、备份和其它容器占用。

### 3.2 被测服务

| 组件 | 当前状态 | 压测判定 |
| --- | --- | --- |
| KC Server | `http://127.0.0.1:7380/readyz` 返回 ready；metrics 可抓取 | 功能拓扑可用 |
| KC profile | `stores.yaml` 为 `profile: local`，Dolt + OpenSearch | 不是 scale deployment profile |
| Dolt | KC 镜像内为 2.3.1；当前实现每次通过 Dolt CLI 执行命令 | 缺独立 `dolt sql-server`、连接池和服务资源隔离 |
| 遗留 Dolt 容器 | `kc-scale-dolt-runtime` 使用 `dolthub/dolt:latest`，仅运行 sleep + CLI wrapper | 不受 Compose 管理、镜像未钉 digest、无 SQL 端口，不能作为资格环境 |
| MySQL | 8.4.8，healthy | `/var/lib/mysql` 使用 tmpfs，只适合功能夹具，不适合持久 soak/恢复测量 |
| OpenSearch | 2.19.3，单节点，512 MiB heap | 只适合本地；不满足 S2 三节点参考档 |
| OpenSearch health | cluster yellow；KC projection 为 8 primary/0 replica，control index 有 1 replica 未分配 | 功能 Search 可用；无冗余、不能做节点故障/容量资格 |
| Gitea | 1.26.3，healthy | 可服务 semantic 功能仓，不是主要规模瓶颈环境 |

当前 Compose 没有 Dolt SQL Server service，也没有专用 load-generator service。KC、
OpenSearch、MySQL 与观测栈共享同一个 8 GiB Docker VM，资源争用无法归因。

### 3.3 数据与负载工具

| 能力 | 实测 | 判定 |
| --- | --- | --- |
| Python runtime | `.venv` Python 3.14.7 | 可用 |
| S0 generator smoke | 成功生成 1,000 table families、100 events；模型为 13,000 objects / 14,000 units | 基础流式生成可用 |
| 固定 seed | 已实现 | 可用 |
| S1–S5 family stream | profile 和流式循环已实现 | 尚未验证大档生成时间、磁盘和 digest |
| H0–H4 历史 | 当前 `--history` 只把 `targetCommits` 写入 `model.json` | `BLOCKED`：未生成 10 万至 2,000 万真实变更 |
| checkpoint/resume | generator 未实现 | `BLOCKED`：大档无法可靠续跑 |
| canonical bytes/histogram/digest | generator 未实现完整证据 | `BLOCKED`：不能校验模型和容量斜率 |
| table family→ChangeSet→Writer | 没有 scale load adapter/runner | `BLOCKED` |
| RPS/并发/到达模型 | 没有 k6/vegeta/wrk/Locust 或仓内等价 runner | `BLOCKED` |
| case 选择与阶段控制 | 没有 `run-load.sh` | `BLOCKED` |
| crash/fault injector | 未实现 | KC-PERF-09/11/12 不可执行 |
| report/capacity collector | 未实现 | 不能生成 `report.json`、`capacity.json` 和 Hmax |

因此“能生成 S0 NDJSON”不等于“能执行 S0 压测”；当前连 S0 端到端 load case 都没有
可调用入口。

### 3.4 可观测与证据链

| 能力 | 当前状态 | 判定 |
| --- | --- | --- |
| Prometheus | ready，`knowledge-catalog` target 为 up | 可用 |
| KC Search 指标 | 原始 histogram 和 P95/P99 recording rule 已有样本 | 可用，但只有 smoke 样本 |
| Jaeger/Loki/Grafana | 均已启动，dashboard smoke 已覆盖 | 功能诊断可用 |
| Prometheus retention | 24 小时 | 不足以安全覆盖“24 小时测量 + warm-up + cool-down + 报告” |
| Loki | 数据目录为 tmpfs | 重启即丢，不能作为资格证据 |
| Jaeger | 当前 Compose 未声明持久证据 volume | 不能保证资格证据留存 |
| 主机/容器资源指标 | 无 node-exporter/cAdvisor 等受管采集 | CPU、RSS、IO、磁盘延迟证据不完整 |
| Dolt SQL/scan counters | 未接入统一 evidence collector | 无法证明热路径扫描禁令 |
| OpenSearch node/index 快照 | 可经 API 手工取得，未纳入 run artifact | 未自动留证 |

审计时 recording rule 返回的 Search P95 约为 1.49 秒，高于 shared-standard 1 秒目标；
该值混合了少量 smoke 和当前环境历史样本，**不能当作压测失败或性能基线**，只证明指标
链路确实在产出数据。

## 4. 用例可执行性映射

| 用例 | 当前可执行性 | 主要阻断 |
| --- | --- | --- |
| KC-PERF-01 Bootstrap | 不可执行 | 无 ChangeSet load runner；无 scale Dolt SQL Server；S2+ 资源不足 |
| KC-PERF-02 稳态 | 不可执行 | 无速率控制、24h runner、独立发压端和长期证据留存 |
| KC-PERF-03/04 burst/shock | 不可执行 | 无 open-loop arrival、queue/event 对账和 backlog collector |
| KC-PERF-05 Canonical 读 | 不可执行 | 无固定 corpus、并发/RPS runner、冷热分组报告 |
| KC-PERF-06 Workspace | 不可执行 | 功能数据存在，但无组合负载 runner 和分成员阶段指标 |
| KC-PERF-07 Search | 不可执行 | Search API 可用，但无稳定 corpus/runner；OpenSearch 仅单节点 512 MiB |
| KC-PERF-08 增量追赶 | 不可执行 | 无历史/lag 注入和 desired/basis artifact collector |
| KC-PERF-09 rebuild | 不可执行 | 无 S2+ 数据、在线故障注入和持久 generation 证据 |
| KC-PERF-10 历史老化 | 不可执行 | H0–H4 真实历史未生成；无 checkpoint/backup/restore harness |
| KC-PERF-11 crash/retry | 不可执行 | 无 run-scoped fault injector 和恢复对照 digest |
| KC-PERF-12 rollover | 不可执行 | 尚无 Hmax、归档容量环境和恢复 harness |

## 5. 达到可执行的最短路径

按依赖顺序完成，不能先扩大硬件再用手工 curl 代替 runner：

1. **补齐 S0 runner 闭环**：case 选择、公开 API 发压、arrival/concurrency、warm-up、
   measure、cool-down、`PASSED|FAILED|INVALID` 和 run-scoped evidence。
2. **补齐规模输入**：把 family stream 翻译为现有 ChangeSet；实现真实 H0–H4 事件历史、
   checkpoint/resume、实际计数、canonical bytes、histogram 和 digest。
3. **建立 scale 服务拓扑**：独立固定版本的 `dolt sql-server`、持久 NVMe volume、连接池、
   单 active-writer lease；独立 load generator；给 KC、Dolt、OpenSearch 设置资源 limit。
4. **补齐性能证据**：延长 Prometheus retention；持久化 trace/log；采集 host/container、
   Dolt SQL/scan、OpenSearch node/index 和 disk latency；自动写入 run artifact。
5. **先过 S0/S1**：验证 runner 自身不饱和、报告可复算、同环境重复 run 方差可接受。
6. **再申请 S2 pilot 环境**：至少满足 16 vCPU/64 GiB Dolt 与 3×16 vCPU/64 GiB
   OpenSearch 参考档；用 S2 实测容量后再推导 S3/S5 集群规模。

未完成第 1–4 项前，本机可以继续做功能开发和观测 smoke，但不应开始 24 小时稳态、
百万表 bootstrap 或 H1 以上历史生成，因为所得数据无法稳定归因或复算。
