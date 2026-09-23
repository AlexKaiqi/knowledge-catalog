# 数仓知识提供方独立压测用例

状态：用例已重构为**性能场景树**（[`.data/scenes/`](scenes/README.md) 的组织方式，视角是
性能测试）；load runner 仍待实现。Dolt 原生规模路线已退役，规模介质为 lakeFS；
总体模型、档位与历史门槛仍以 [`docs/SCALE_BENCHMARK.md`](../../docs/SCALE_BENCHMARK.md) 为准，
其 §10 数值门槛重登记前只作默认参考。

本文件不再是逐条用例的唯一清单。用例本体（状态、construct、探针、量级阶梯、指标、
清理与 runner 能力需求）按场景树组织在 [`scenes/`](scenes/README.md)；本文件保留四件
仍然全局生效的事：**单次运行合同**、**硬门禁**、**旧 KC-PERF 映射**、**执行分级**。

环境不是本树的一部分：执行器只读取环境配置（[`scenes/env.example.yaml`](scenes/env.example.yaml)
的形状）并绑定一个已部署、已就绪的 KC 环境；**不启动、不编排任何容器或服务**。
测试阶段由人起好本地栈后把 endpoint 填进环境配置；同一棵树最终对生产环境出结论。
环境要求与就绪性见 [`ENVIRONMENT.md`](ENVIRONMENT.md)。

## 1. 树总览

```bash
python3 .data/scale/scenes/perf_tree.py          # 树与探针
python3 .data/scale/scenes/perf_tree.py --check  # 结构检查
```

```text
environment-bound/                        # 环境配置绑定 + readiness（根前态）
├─ probe-idle-baseline                    # 空载基线（为全部曲线扣除环境噪声）
└─ scale-repository-empty/                # 专用规模仓已建且为空
   ├─ probe-upload-filecount-curve        # 多文件上传量级曲线（写入主曲线）
   ├─ probe-object-size-curve             # 对象大小阶梯
   ├─ probe-bulk-load-batch-curve         # 导入批大小曲线
   └─ bulk-loaded/                        # 档位数据已导入（计数/digest 核对）
      ├─ probe-read-latency-curve         # 点读/批量读/resolve 时延曲线
      ├─ probe-browse-list-curve          # 浏览分页随文件数量曲线
      ├─ probe-write-after-load           # 已加载仓上的写入退化
      ├─ probe-storage-accounting         # 存储容量账
      ├─ probe-workspace-combine-curve    # 多仓组合读
      ├─ dataset-published/               # 交付 Dataset 已发布（整前缀冻结）
      │  └─ probe-clone-delivery-curve    # dataset clone 物化吞吐与分页曲线
      ├─ history-aged/ [gap]              # 真实历史到档（生成器历史未实现）
      │  └─ probe-history-degradation-curve
      └─ projection-synced/               # 投影追平固定 HEAD
         ├─ probe-search-latency-curve    # 检索时延曲线
         ├─ probe-index-rebuild-curve     # 全量索引重建速度
         ├─ probe-index-update-latency-curve  # 索引更新时延（commit→可见）
         └─ steady-soaked/                # 稳态窗口已运行
            ├─ probe-steady-soak          # 稳态判定（RSS/滞后/时效）
            ├─ probe-burst-backlog        # 日峰与突发积压
            ├─ probe-shock-silence        # 瞬时冲击与静默恢复
            └─ probe-failure-recovery     # 负载下故障恢复
```

指标口径以 [`scenes/metrics.yaml`](scenes/metrics.yaml) 为唯一登记表：上传吞吐、文件
数量曲线、访问速度、索引速度、索引更新时延、检索、容量资源与故障恢复六族，
每条指标声明单位、曲线维度、采集方式与门槛引用。

## 2. 旧 KC-PERF 映射

| 旧 ID | 内容 | 现归属 |
| --- | --- | --- |
| KC-PERF-01 | Bootstrap 容量与批大小 | `bulk-loaded` construct（构建证据）+ `scale-repository-empty/probe-bulk-load-batch-curve`（批大小曲线） |
| KC-PERF-02 | 稳态写入与混合读取 | `steady-soaked` construct + `probe-steady-soak`；索引侧归 `projection-synced/probe-index-update-latency-curve` |
| KC-PERF-03 | 日峰与 100x burst | `steady-soaked/probe-burst-backlog` |
| KC-PERF-04 | Shock 与低流量恢复 | `steady-soaked/probe-shock-silence` |
| KC-PERF-05 | Canonical 读容量 | `bulk-loaded/probe-read-latency-curve` + `probe-browse-list-curve` |
| KC-PERF-06 | 多 Repository Workspace 消费 | `bulk-loaded/probe-workspace-combine-curve` |
| KC-PERF-07 | Search 混合负载 | `projection-synced/probe-search-latency-curve` |
| KC-PERF-08 | Projection 增量追赶 | `projection-synced/probe-index-update-latency-curve`；断连恢复的故障面归 `steady-soaked/probe-failure-recovery` |
| KC-PERF-09 | 在线全量 rebuild | `projection-synced/probe-index-rebuild-curve` |
| KC-PERF-10 | 历史老化与幂等账本 | `history-aged/probe-history-degradation-curve`（gap：生成器历史未实现） |
| KC-PERF-11 | 负载下 crash/retry | `steady-soaked/probe-failure-recovery` |
| KC-PERF-12 | Generation rollover 与归档恢复 | gap：待 REVIEW-01 选定代际方案后补状态，不预先建目录 |

原用例中 Dolt 专属描述（SQL server、command ledger、`kc_files` 等）随路线退役作废；
行为正确性归 `.data/scenes` 与共享合同，压测只保留停止条件与结果可信度门禁。

## 3. 单次运行合同

每次执行必须显式记录以下参数；缺一项则结果为 `INVALID`，不能判为通过：

| 类别 | 必填参数 |
| --- | --- |
| 被测版本 | KC revision、Go、快照介质（lakeFS/对象存储）与索引引擎（OpenSearch）exact version 或 image digest、schema/layout version |
| 环境 | CPU、RAM、磁盘与文件系统、节点数、容器/资源限制、网络 RTT、时钟同步状态 |
| 数据 | `S*`、`H*`、seed、对象/单元/endpoint/eligible-doc 实际数量、canonical bytes |
| 负载 | 到达模型、目标速率、读写比例、并发、page/batch size、预热和测量时长、量级阶梯取值 |
| Provider | 快照介质连接池与 durability；OpenSearch shard/replica/refresh/mapping digest |
| 基线 | 同硬件、同数据档、同负载口径的已批准 run；没有基线时标记 `BASELINE` |

### 3.1 通用执行阶段

1. `bind`：读取环境配置并完成 readiness/版本/授权核验（`environment-bound` construct）。
2. `prepare`：建立隔离前态，生成或恢复固定档位，校验实际模型计数和 digest。
3. `warm-up`：仅填充连接池、页缓存与目标缓存；样本单独保存，不进入判定。
4. `measure`：按探针施加固定负载与量级阶梯；禁止途中改变硬件、replica、refresh、
   durability、数据模型或历史开关。
5. `cool-down`：停止新流量，继续观察 backlog、projection lag、GC 与资源回落。
6. `verify`：invariant/differential digest、抽样固定 basis 回读和证据完整性检查。
7. `report`：生成 `report.json`，逐项给出 `PASSED`、`FAILED` 或 `INVALID`，不只给图表。

默认预热 10 分钟；时延类探针默认连续测量 30 分钟；`phases` 另有声明时以探针为准。
发压端 CPU 持续达到 70%、时钟漂移、采样缺口、数据量不符或被测配置在测量中变化，
本次结果为 `INVALID`。

### 3.2 通用硬门禁

任一探针出现以下情况立即停止加压并判 `FAILED`：

- source event 丢失、duplicate 产生额外 revision、partial Canonical commit；
- wrong-basis hydration、continuation 跨 pin/query 被接受、Repository ref/CAS 破坏；
- Writer 同步等待索引引擎，或普通热路径出现全仓扫描；
- OOM、磁盘利用率超过 85%、持续 I/O error；
- 指标、trace、scan counter 或最终 digest 无法相互解释。

## 4. 执行分级

| 级别 | 探针与档位 | 触发 |
| --- | --- | --- |
| PR | 不运行本树；只运行功能/组件门禁与 S0 生成 smoke | 相关改动 |
| Nightly | `probe-read-latency-curve` / `probe-search-latency-curve`，S1-H0 | 专用 runner |
| Weekly | 上传/读/检索/索引更新四条曲线，S2-H1 | lakeFS/OpenSearch 集群 |
| Release candidate | 除 history-aged 外全部探针，S3-H2/H3 | 容量环境 |
| Qualification | 全部探针的各自资格档，覆盖 S5 与 H4 | 首次上线或规模组件重大变化 |

介质 major、layout major、OpenSearch mapping/shard policy、硬件或 durability 变化会使
旧基线失效，必须重新建立相应档位的 `BASELINE`，不能跨环境沿用绝对吞吐结论。

## 5. 结果目录与最小报告

```text
.data/scale/runs/<run-id>/
├── manifest.json
├── model.json
├── timeline.jsonl
├── metrics/
├── traces/
├── correctness.json
├── capacity.json
└── report.json
```

`report.json` 除总体结果外，至少写入 `caseId`（探针 id + 路径）、档位、量级阶梯取值、
声明并发/速率、有效样本窗口、每个 gate 的实测值和证据路径。`FAILED` 表示被测系统
未过门槛；`INVALID` 表示环境或证据不足，二者不得混用。原始 metrics、trace 和规模
生成物不提交仓库。
