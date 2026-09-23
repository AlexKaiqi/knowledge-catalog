# 性能场景树

压测用例按**场景树**组织：目录是共享测量前态，`_build` 建立状态，`_probes` 从状态出发
做一次测量。它借用 [`.data/scenes/`](../../scenes/README.md) 协议旅程树的组织方式，
但视角不同：协议树回答「行为是否正确」，本树回答「在声明的数据量、并发和持续时间下，
吞吐、时延、资源、追赶和恢复如何随量级变化」。总体规模模型、档位与历史门槛仍以
[`docs/reviewed/scale-benchmark.md`](../../docs/reviewed/scale-benchmark.md) 为准；环境配置合同见
[`ENVIRONMENT.md`](../ENVIRONMENT.md)；指标唯一登记表是本目录
[`metrics.yaml`](metrics.yaml)。

旧 `KC-PERF-01`–`12` 扁平清单已重构进本树，映射见
[`CASES.md`](../CASES.md)。

## 1. 本树自己的不变式

1. **只绑定环境，不自建环境**。执行器只读取环境配置（`env.example.yaml` 的形状），
   指向一个**已部署、已就绪**的 KC 环境；不启动、不编排、不等待任何容器或服务。
   测试阶段由人用现有本地栈起好服务，把 endpoint 填进环境配置；同一棵树最终对生产
   环境出结论，只是换一份环境配置与凭证引用。
2. **只经公开 surface 施压**：Writer、Knowledge（read/search/resolve/relations/log）、
   Dataset 交付、Operations 只读观测。不直连快照介质、不直写 OpenSearch、
   不新增测试专用 Write Surface。
3. **状态是共享前态，探针是独立测量**。construct 建立可复用的测量前态并记录构建
   证据；每条 probe 从该前态独立执行，自己创建的 scratch 仓/分支在 `cleanup` 里声明
   清理，不被后继状态或其它探针继承。
4. **一条探针回答一个测量问题，结论是曲线不是单点**。曲线自变量用 `ladder` 声明
   （量级阶梯），阶梯定义在环境配置 `profile.ladders`，缺省值见
   [`env.example.yaml`](env.example.yaml)。
5. **指标 id 只在 `metrics.yaml` 登记**。探针与 construct 引用登记过的 id；
   数值门槛引用 `docs/reviewed/scale-benchmark.md` §10 / §14，重登记前只作默认参考
   （见 §6 资格状态）。
6. **凭证只以环境变量引用名出现**。环境配置不出现密钥值；引用名经
   `perf_tree.py --env` 校验形状。
7. 压测不进 `make test`，不属于协议场景验收。压测中的正确性检查只是停止条件与
   结果可信度门禁，不重复维护一套业务功能场景。

## 2. 目录格式

| 路径 | 含义 |
|---|---|
| 不以 `_` 开头的状态目录 | 共享测量前态；目录嵌套即构建前置（单父） |
| `_meta.yaml` | 本状态自描述：`title` / `fixture`（本步交付的测量条件）/ `views` / construct 需要的 runner 能力 `requires` / construct 顺带记录的 `metrics` / 明确 `gaps` |
| `_build/construct.yaml` | 从父状态进入本状态的操作计划：`summary` / `steps`（`surface` + `action` + `params`）/ `metrics`（构建证据，不是资格曲线）/ `requires` |
| `_probes/<probe>.yaml` | 独立测量：`question` / `ladder` / `operation` / `metrics` / `correctness` / `isolation` + `cleanup` / `requires` |
| `_views.yaml` | 工程关注点视图定义（write / read / index / search / capacity / resilience） |
| `metrics.yaml` | 指标登记表：id、单位、曲线维度、采集方式、门槛引用 |
| `env.example.yaml` | 环境配置模板（形状合同；真实配置不提交） |
| `perf_tree.py` | 树阅读与结构检查工具 |

所有 YAML 使用 README/metrics 相同的受限子集：缩进映射、`- ` 标量列表、引号标量；
不支持多键列表项、flow 风格与多行标量。

## 3. 环境配置合同

一份环境配置描述一个被测环境。执行器 `bind` 时完成：连通与 readiness 校验、
版本/拓扑记录、主体与授权核验、档位与阶梯声明核对；全部通过才算进入
`environment-bound` 前态。字段含义与校验规则见 [`env.example.yaml`](env.example.yaml)
注释与 [`ENVIRONMENT.md`](../ENVIRONMENT.md)。

- 测试阶段：本地栈由人启动（不新增编排放进本树），`serverURL` 指向本地。
- 生产阶段：同一份树、另一份环境配置；凭证走环境变量引用，原始值不进仓库。
- 证据写入 `.data/scale/runs/<run-id>/`（gitignore），格式沿用
  `docs/reviewed/scale-benchmark.md` §13。

## 4. 执行合同（runner）

runner 尚未实现；每条 probe 的 `requires` 列出它需要的能力，合起来就是实现路线：

| 能力 | 含义 |
|---|---|
| `arrival-models` | open/closed loop 到达、steady/diurnal/burst/shock 速率控制 |
| `long-window` | ≥24h 持续测量与采样留存（含 cool-down） |
| `fault-injection` | 在声明边界注入 crash / 断连 / 超时，并恢复对照 |
| `history-generation` | 生成真实 H0–H4 变更历史（依赖生成器历史支持） |
| `checkpoint-resume` | 大档导入/重建可断点续跑 |
| `multi-repo` | 第二个仓与 Workspace/Dataset 组合定义与清理 |
| `resource-sampling` | 发压端与被测端 CPU/RSS/IO/disk 样本采集 |
| `server-metrics` | 只读抓取 Prometheus 指标与运维快照（可选增强） |

单次运行阶段、停止条件、`PASSED/FAILED/INVALID` 判据沿用 `CASES.md` §2 与
`docs/reviewed/scale-benchmark.md` §7/§14：预热只填缓存不判定；测量中禁止改变被测配置；
发压端饱和、采样缺口、数据量不符、正确性门禁失败即停止或判无效。

## 5. 树与阅读

```bash
python3 .data/scale/scenes/perf_tree.py              # 文本树
python3 .data/scale/scenes/perf_tree.py --json       # 结构化
python3 .data/scale/scenes/perf_tree.py --view write # 单个关注点视图
python3 .data/scale/scenes/perf_tree.py --metrics    # 指标登记表
python3 .data/scale/scenes/perf_tree.py --env my-env.yaml   # 校验环境配置
python3 .data/scale/scenes/perf_tree.py --check      # 结构与引用检查
```

## 6. 资格状态

- 本树当前是**用例规格**：runner 未实现，没有任何一条探针产生过资格结论。
- `docs/reviewed/scale-benchmark.md` §10 的数值门槛按 native Dolt 路线编写；该路线已退役，
  介质为 lakeFS，资格线与实测入口待按 `docs/STORE_ADAPTERS.md` 重新登记。重登记前
  这些数值只作默认参考，不构成 lakeFS 部署的承诺或通过线。
- `history-aged` 节点依赖真实历史档生成（`history-generation`），生成器未实现前
  该节点不可构建；`_meta.yaml` 的 `gaps` 显式记录。代际 rollover 结论待 REVIEW-01
  选定方案后补充，不预先建状态。

## 7. 维护

1. 新增指标先登记 `metrics.yaml`（id、单位、维度、采集方式），再在探针里引用。
2. 新增状态目录必须同时有 `_meta.yaml` 与 `_build/construct.yaml`；探针文件名即 id。
3. 改动后运行 `perf_tree.py --check`；视图标签必须让视图并集覆盖全部节点与探针。
4. 协议行为问题归 `.data/scenes`；不要把功能回归伪装成压测探针，也不要在探针里
   钉协议后态。
