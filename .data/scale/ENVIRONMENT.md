# 压测环境：配置合同与就绪性

> 状态：Dolt 原生规模路线已退役；规模介质为 lakeFS，档位资源要求待按
> `docs/SCALE_BENCHMARK.md` 与 `docs/STORE_ADAPTERS.md` 重新登记。§5 的 2026-08-31
> 审计保留为历史记录，其中 Dolt 专属条目不再代表当前部署。

压测用例按场景树组织在 [`scenes/`](scenes/README.md)，用例入口与运行合同见
[`CASES.md`](CASES.md)。本文回答一件事：**一份环境配置怎样才算绑定了可测环境**。

## 1. 环境配置合同

- 环境配置是压测与被测环境之间的**唯一入口**，形状以
  [`scenes/env.example.yaml`](scenes/env.example.yaml) 为准；形状校验：
  `python3 .data/scale/scenes/perf_tree.py --env <file>`。
- **执行器不启动、不编排、不等待任何容器或服务**。被测环境必须已部署、已就绪；
  测试阶段由人用现有本地栈起好服务后把 endpoint 填进配置，最终对生产测试时
  换一份配置与凭证引用，用例树不变。
- 凭证只以环境变量引用名（`credentialsEnv`）出现，引用值不进配置、不进仓库、
  不进证据。
- 每次运行使用独立 `runId` 与证据目录 `.data/scale/runs/<run-id>/`（gitignore）；
  配置文件本身不提交，随 run 归档一份副本入 manifest。
- 探针创建的 scratch 仓统一使用 `repositoryPrefix` 前缀；`cleanup.scratchRepositories`
  声明 run 结束后的处置（缺省 delete），retain 必须同时记录责任人与期限。

## 2. bind 就绪门禁

以下全部通过才构成场景树的 `environment-bound` 前态；任一失败即 bind 失败，
不进入测量，也不把"服务能启动"当成"环境可测"：

| 门禁 | 内容 |
| --- | --- |
| 连通 | `serverURL` 可达且 readiness 通过；observability URL（如填）只读可达 |
| 身份 | `principal` 存在，登录成功，`whoami` 一致；所需 grants 覆盖本 run 探针声明面 |
| 目标 | `catalog` 可见；`setupAllowCreate=false` 时既有空仓已就位且为空 |
| 版本 | 被测 revision/版本、介质与索引引擎版本、OS/资源限制记录入 manifest；不可得项标 `absent` |
| 档位 | `profile.scale` / `profile.history` 与 `profile.ladders` 覆盖值留档；与历史基线口径冲突时显式声明 |
| 隔离 | scratch 前缀无残留仓；证据目录可写且为空 run |
| 发压端 | 发压进程与被测服务分进程（正式 S2 以上分机/分容器并限制资源）；CPU <70% |

`READY` 只表示该 run 可产出可复算证据；发压端饱和、采样缺口、配置漂移和数据量
不符在测量中出现时结果判 `INVALID`（见 `CASES.md` §3）。

## 3. 资源参考档（待重登记）

以下参考档沿自旧设计，介质中立转写；**数值未经 lakeFS 部署实测校准前只作申请
资源的起点，不构成资格线**：

- pilot（S2）：快照介质节点 16 physical vCPU / 64 GiB RAM / 本地 NVMe，容量至少为
  预测 target bytes 的 3 倍；OpenSearch 至少 3 个 data node（每 node 16 vCPU /
  64 GiB）；Writer 与发压端分机或分容器。
- qualification（S5/H4）：从 64 vCPU / 256 GiB RAM 与企业 NVMe 起步；最终容量按
  S2/S3 实测 `bytes/object`、`primary bytes/doc`、备份与 rebuild 临时空间推导；
  磁盘利用率目标不超过 60%，并容纳新旧 generation 共存。
- H 档必须由真实变更提交构成，禁止空 commit 或只在 manifest 声明目标数。

正式资源与资格线由 `docs/SCALE_BENCHMARK.md` 重登记拥有；重登记前 §2 的门禁与
`CASES.md` §3 的合同优先。

## 4. 可观测与证据留存

- Prometheus 指标抓取为可选增强（`observability.prometheusURL`，只读）；无该配置时
  以 client-timer 与 resource-sample 为准，探针 `collect: server-metrics` 的指标改记
  `absent` 并在报告标注。
- 证据保留时间必须长于最长探针窗口（含 warm-up/cool-down 与报告复算），临时目录
  （tmpfs）不能作为资格证据存放地。
- 磁盘水位、发压端 CPU、时钟同步状态进入 manifest 与 timeline，与指标相互解释。

## 5. 附录：2026-08-31 环境实测（历史记录）

> 以下审计针对当时的 Dolt 路线 Compose 拓扑，仅证明"当时连 S0 端到端 load case
> 都没有可调用入口"，不描述当前 lakeFS 部署；工具链缺口（runner、历史生成、
> checkpoint/resume、证据采集）在 runner 实现前仍然成立，逐项能力需求见
> `scenes/README.md` §4 与各探针 `requires`。

结论：**当时环境不具备执行 `KC-PERF-01`–`KC-PERF-12` 并产出有效容量结论的条件。**

| 层级 | 当前结论（2026-08-31） |
| --- | --- |
| 功能拓扑（KC/存储/索引/观测连通） | `READY`，仅限本地功能用途 |
| 压测工具链（速率/并发/阶段/报告） | `BLOCKED` |
| S1/nightly | `BLOCKED` |
| S2/pilot（三节点 OpenSearch、资源隔离） | `BLOCKED` |
| S3/release candidate（隔离容量环境） | `BLOCKED` |
| S5/H4 qualification（过亿对象、2,000 万真实提交） | `BLOCKED` |

要点摘录：Docker VM 约 7.65 GiB 且无资源 limit，无法形成可复现基线；OpenSearch 单节点
512 MiB heap；Prometheus retention 24h 不足以覆盖 24h 测量；Loki/Jaeger 数据易失；
generator 可产出 S0 NDJSON 但没有 load runner、真实 H 历史、checkpoint/resume、
canonical bytes/digest 证据与 fault injector。完整表格与逐项判定见当时版本
（git 历史）的本文 §3–§4。
