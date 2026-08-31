# 数仓知识提供方独立压测用例

状态：待 load runner 实现

本文件是压测用例的唯一清单。总体规模模型、事件分布、指标定义和资格线仍以
[`docs/SCALE_BENCHMARK.md`](../../../docs/SCALE_BENCHMARK.md) 为准；这里把它们组织成
可以独立准备、执行、判定和留证的用例。

压测不属于 `features/*.feature` 的功能验收，也不由 Behave 执行。功能验收回答行为
是否正确，压测回答在声明的数据量、历史量、并发和持续时间下，吞吐、时延、资源、
追赶和恢复是否达标。压测中的正确性检查只是停止条件和结果可信度门禁，不重复维护
一套业务功能场景。

## 1. 执行边界

- 执行压测前，目标 revision 必须已通过 `make test` 和
  `make test-data-warehouse`；两者的结果只作为前置证据，不计入压测耗时。
- 每条用例创建独立的 `<run-id>`、Repository、投影 generation 和证据目录。用例可从
  只读规模 checkpoint 克隆起点，但不得消费上一条用例运行后留下的可变状态。
- 压测只通过公开 Writer、Knowledge、Catalog、Operations 和 Retrieval surface 施压；
  不用一次性 CLI 直开 Home，不绕过 Server 写 Dolt/OpenSearch，也不新增测试专用
  Write Surface。
- `S0`–`S5`、`H0`–`H4`、事件混合、Canonical read mix 和 Search mix 均沿用
  `docs/SCALE_BENCHMARK.md`。用例只声明选用哪组参数，不另造第二套模型。
- 业务模型、事件流和负载配置位于本目录；原始输出写入
  `.data/data-warehouse/runs/scale/<run-id>/`，均不提交。
- `make test-data-warehouse` 不运行本清单。未来 load runner 应提供单用例入口，例如
  `run-load.sh KC-PERF-03`，但在 runner 实现前不得把示例入口写进 Makefile 冒充可执行能力。

## 2. 单次运行合同

每次执行必须显式记录以下参数；缺一项则结果为 `INVALID`，不能判为通过：

| 类别 | 必填参数 |
| --- | --- |
| 被测版本 | KC revision、Go、Dolt、OpenSearch、镜像 digest、schema/layout version |
| 环境 | CPU、RAM、磁盘与文件系统、节点数、容器限制、网络 RTT、时钟同步状态 |
| 数据 | `S*`、`H*`、seed、对象/单元/endpoint/eligible-doc 实际数量、canonical bytes |
| 负载 | 到达模型、目标速率、读写比例、并发、page/batch size、预热和测量时长 |
| Provider | Dolt 连接池与 durability；OpenSearch shard/replica/refresh/mapping digest |
| 基线 | 同硬件、同数据档、同负载口径的已批准 run；没有基线时标记 `BASELINE` |

### 2.1 通用执行阶段

1. `prepare`：创建隔离环境，生成或恢复固定档位，校验实际模型计数和 digest。
2. `warm-up`：仅用于填充连接池、JIT/页缓存和目标缓存；样本单独保存，不进入判定。
3. `measure`：按用例施加固定负载，禁止途中改变硬件、replica、refresh、durability、
   数据模型或历史开关。
4. `cool-down`：停止新流量，继续观察 backlog、projection lag、GC 和资源回落。
5. `verify`：执行 invariant/differential digest、抽样固定 basis 回读和证据完整性检查。
6. `report`：生成 `report.json`，逐项给出 `PASSED`、`FAILED` 或 `INVALID`，不只给图表。

默认预热 10 分钟；时延类用例默认连续测量 30 分钟。用例另有持续时间时以用例为准。
生成器 CPU 持续达到 70%、时钟漂移、采样缺口、数据量不符或被测配置在测量中变化，
本次结果为 `INVALID`。

### 2.2 通用硬门禁

任一用例出现以下情况立即停止加压并判 `FAILED`：

- source event 丢失、duplicate 产生额外 revision、partial Canonical commit；
- wrong-basis hydration、continuation 跨 pin/query 被接受、Repository ref/CAS 破坏；
- Writer 同步等待 OpenSearch，或普通热路径出现全仓扫描；
- OOM、磁盘利用率超过 85%、持续 I/O error；
- 指标、trace、SQL scan counter 或最终 digest 无法相互解释。

## 3. 用例总览

| ID | 用例 | 主档位 | 核心结论 |
| --- | --- | --- | --- |
| KC-PERF-01 | Bootstrap 容量与批大小 | S1→S2→S3→S5 | 当前态能否有界导入并形成可服务 basis |
| KC-PERF-02 | 稳态写入与混合读取 | S3-H0 / S5-H0 | 日常变更下 Receipt、Search 可见性和读延迟 |
| KC-PERF-03 | 日峰与 100x burst | S3-H0 / S5-H0 | 峰值吞吐、排队和停止后追平能力 |
| KC-PERF-04 | Shock 与低流量恢复 | S3-H0 | 瞬时积压及零等待合并是否引入额外延迟 |
| KC-PERF-05 | Canonical 读容量 | S1/S2/S3/S5 | 点读、批读、关系、历史读和分页的延迟/斜率 |
| KC-PERF-06 | 多 Repository Workspace 消费 | S2-H0 | 组合读取、pin 与 continuation 的额外成本 |
| KC-PERF-07 | Search 混合负载 | S2/S3/S5 | 完整 Search 的时延、吞吐与候选放大 |
| KC-PERF-08 | Projection 增量追赶 | S3-H0 | 落后、断连和重启后能否追到固定 desired target |
| KC-PERF-09 | 在线全量 rebuild | S2→S3→S5 | rebuild 与写/搜并行时的时长、资源和可用性 |
| KC-PERF-10 | 历史老化与 command ledger | S2/S3 + H0→H4 | commit 深度对启动、读、diff、log、幂等的退化 |
| KC-PERF-11 | 负载下 crash/retry | S2-H1 | 关键故障窗口下的恢复时间和重复工作量 |
| KC-PERF-12 | Generation rollover 与归档恢复 | S3 + H3/H4 | 切代阈值、恢复时间和新 generation 性能 |

## 4. 详细用例

### KC-PERF-01 Bootstrap 容量与批大小

**目的**：确定从墙外 current state 建立 native Dolt Repository 和首个可服务投影的
吞吐、资源上界及最佳批大小。

**前置**：空 Repository；固定 seed；S1、S2、S3 依次执行，S3 通过后才执行 S5。

**负载**：

1. 分别以每 commit 500、1,000、5,000 个 table families 导入相同档位；
2. source page 可并发拉取，Canonical 只保留单 active writer；
3. bootstrap 期间持续注入 steady event，并在初始遍历后追赶到固定 watermark；
4. 发布后执行冷/热 Read、ReadMany、Relation page 和 Search 抽样。

**观测**：families/s、units/s、bytes/s、commit P50/P95/P99、catch-up duration、峰值 RSS、
临时磁盘、rows/bytes per table、Dolt/OpenSearch 写放大、可服务时间。

**通过标准**：

- 实际对象、unit、endpoint、eligible-doc 数和生成器 manifest 一致；
- RSS 与 page/batch size 有界，不随总对象数线性物化；
- steady event 全部进入最终 basis，Writer 不等待投影完成；
- S5 candidate bootstrap + catch-up 不超过 48 小时；
- 所有通用硬门禁通过。S1–S3 不能外推替代 S5 资格结果。

**证据**：`model.json`、逐批 timeline、Dolt/OpenSearch 容量快照、RSS/IO 原始样本、
最终 digest、三种批大小的对照表。

### KC-PERF-02 稳态写入与混合读取

**目的**：验证日常变化持续到达时，立即 commit、Canonical 读取和 Projection 追赶能否
共同满足时效且资源不爬升。

**前置**：S3-H0；资格运行使用 S5-H0；Projection 已追到起始 commit。

**负载**：连续至少 24 小时注入 10,000 events/day 的 steady 事件混合；每个逻辑事件
立即 commit。同时施加 Canonical read mix 和 Search mix。5% duplicate、2% out-of-order
按模型注入，不暂停正常读取。

**观测**：event received→Receipt、received→Search visible、events/commit、read/search
P50/P95/P99、projection commit/wall-clock lag、CPU、RSS、GC、IO、ledger pending count。

**通过标准**：

- received→Receipt P99 ≤ 2 秒；received→Search visible P99 ≤ 5 秒；
- 低流量事件 batching wait 为 0；duplicate 不产生额外 revision；
- current Read P99 ≤ 100 ms，ReadMany(100) P99 ≤ 500 ms；
- 24 小时内 RSS 不随对象或 commit 总数线性爬升；
- 同口径 P95 相对已批准基线退化不超过 25%。

**证据**：完整 24 小时 arrival/timeline、原始 histogram、资源曲线、projection lag、
事件与 commit/Receipt 对账摘要。

### KC-PERF-03 日峰与 100x burst

**目的**：确定短时高峰是否导致拒绝、无界排队或无法在目标时间内追平。

**前置**：S3-H0，并具备同环境 steady 基线；基线可在本 run 的独立前置窗口生成，
不读取 KC-PERF-02 的可变运行状态。资格运行使用 S5-H0。

**负载**：先以 steady 运行 30 分钟，再以平均速率 10x 运行 2 小时；恢复 steady
30 分钟后，以至少 12 events/s 运行 30 分钟，最后停止新事件并保留读/Search 负载直到
backlog 清零。严格单-event commit 和零等待 queue coalescing 分两个独立 run 比较。

**观测**：持续 intake、queue depth、oldest event age、events/commit、lease wait、commit
P99、backlog drain、Search lag、读延迟、拒绝/重试和资源饱和点。

**通过标准**：

- 100x 阶段持续 intake ≥ 12 events/s，维持 30 分钟；
- burst 结束后 backlog ≤ 10 分钟清零；
- coalescing 不使用固定时间窗，所有 event ID 都进入 commit/证据；
- backlog 清零后时延恢复到同 run 的 steady 区间，且通用硬门禁全部通过。

**证据**：各阶段边界、queue/lag 时间线、两种 commit 策略对照、最老/最新事件等待时间。

### KC-PERF-04 Shock 与低流量恢复

**目的**：验证极短时间突发不会破坏队列、幂等和低流量零等待语义。

**前置**：S3-H0，系统在 steady 基线稳定 30 分钟。

**负载**：10 秒内送入 1,000 events，其中保留 5% duplicate 和 2% out-of-order；随后
不再制造 backlog。清零后静默 5 分钟，再发送单个事件。

**观测**：接收成功数、峰值 queue、oldest age、drain duration、events/commit、重复事件
命中 ledger 的延迟，以及静默后单事件的 queue/commit/Receipt 时间。

**通过标准**：

- 1,000 个逻辑事件全部有可对账结论，丢失为 0；
- duplicate 额外 revision 为 0，out-of-order 不推进错误 checkpoint；
- backlog 最终清零且 Projection 追到同一固定 target；
- 静默后单事件 batching wait 为 0；
- drain duration 作为容量结果报告，不与 KC-PERF-03 的持续 burst 混成一个吞吐数字。

**证据**：逐事件状态摘要、queue 时间线、ledger replay 统计、静默后单事件 trace。

### KC-PERF-05 Canonical 读容量

**目的**：独立测量 native Canonical 的时延、吞吐和随数据量增长的斜率，不混入写入或
OpenSearch rebuild。

**前置**：只读固定 pin；S1、S2、S3、S5 使用相同 seed 和 query corpus；每档分别冷启和
预热。目标并发和目标 RPS 必须在 run manifest 中声明，结果只资格化该并发，不外推。

**负载**：按 Canonical mix 运行 30 分钟：Read 40%、ReadAddress 15%、ReadMany 15%、
Resolve 10%、Relation page 10%、bounded Repository maintenance/export page 5%、
Schema/AccessSpec 3%、Log/Diff/Provenance 2%。70% 命中 1% hot objects，25% 均匀随机，
5% 读取历史 commit；maintenance/export page 不得伪装成公开 Knowledge LIST。

**观测**：各操作和冷热分组的 throughput、P50/P95/P99、rows scanned/returned、cache
hit/miss、continuation bytes、连接池等待、CPU/RSS/IO。

**通过标准**：

- current Read P99 ≤ 100 ms；historical Read P99 ≤ 250 ms；
- ReadMany(100) P99 ≤ 500 ms；Relation page(100) P99 ≤ 250 ms；
- Schema list P99 ≤ 250 ms；object Log(limit=20) P99 ≤ 500 ms；
- from→to 10,000 commits gap 的 changed objects P99 ≤ 1 秒；
- page iterator 的 RSS 与 page size 有界，热路径全仓扫描计数为 0。

**证据**：按操作/冷热/档位拆分的 histogram、SQL query/scan 统计、continuation 和 RSS
样本、S1→S5 斜率。

### KC-PERF-06 多 Repository Workspace 消费

**目的**：量化 physical + semantic 两成员组合、一次性 pin 解析和跨成员分页的额外成本。

**前置**：physical 使用 S2-H0；semantic objects 为 physical tables 的 1%，另做 10%
stress；WorkspaceDefinition 固定且两个成员均已授权。

**负载**：30 分钟内循环 Workspace resolve、跨 member Read/ReadMany、同 object_id
联邦读、连续 10 页 list/relations，以及带 continuation 的 Search。测量中并发推进成员
HEAD，但每个请求继续使用开始时解析的 pin。

**观测**：resolve latency、每成员 latency、总请求 P50/P95/P99、continuation bytes、
成员 fan-out、授权检查和 pin cache 命中。

**通过标准**：

- 单请求只解析一次 selector，测量中 HEAD 推进不改变本次 basis；
- continuation 跨 pin/query 复用成功数为 0；
- 每成员回读都使用结果声明的固定 commit，wrong-basis 为 0；
- 同负载相对单 Repository 基线的额外开销被单独报告，不掩入 Provider 延迟。

**证据**：Workspace pin、分成员 trace、continuation 负例计数、单仓/双仓对照 histogram。

### KC-PERF-07 Search 混合负载

**目的**：确定完整 Search 在目标 eligible-doc 数和声明并发下的延迟、吞吐、候选放大与
Canonical hydrate 成本。

**前置**：S2、S3、S5 的 active generation 已追到固定 basis；query corpus 在各档使用
同一分布。并发、目标 RPS、shard/replica/refresh 必须显式记录。

**负载**：AllTerms/AnyTerms/Phrase、typed EQ/IN/NEQ/EXISTS/MISSING、number/time range、
string prefix、sort + continuation 按固定 corpus 运行 30 分钟；limit 为 10/20/100，包含
高候选和零候选查询。只统计 `outcome=ok` 且 `completeness=complete` 的主延迟分位数。

**观测**：完整请求和 plan/probe/hydrate/orchestration 阶段 P50/P95/P99、QPS、候选数、
hydrate 数、candidate amplification、OpenSearch CPU/heap/GC/disk、Canonical 回读时延。

**通过标准**：

- `shared-standard` 口径下完整 Search P95 ≤ 1 秒、P99 ≤ 3 秒；
- 同 profile 的 P95 相对已批准基线退化不超过 25%；
- CandidateRef 与 Canonical hydrate basis 一致，wrong-basis 为 0；
- partial、拒绝和输入错误单独统计，不得通过剔除慢成功请求美化分位数。

**证据**：query corpus digest、原始 histogram、阶段分解、候选放大分布、OpenSearch 与
Canonical 关联 trace。

### KC-PERF-08 Projection 增量追赶

**目的**：验证通知丢失、长时间落后、Provider 断连和 controller 重启时，增量投影仍能
从固定 basis/desired target 恢复且不拖慢 Writer。

**前置**：S3-H0 active generation 与 Canonical 对齐。

**负载**：依次执行 1 object/commit、1 family/commit、跨 1/100/10,000 commits 的 net
diff；再让 Projection 分别落后 1 小时和 24 小时。最后断开 OpenSearch 30 分钟并持续
执行 KC-PERF-02 steady 写入，恢复后继续施加 Search。

**观测**：desired target、basis、commit/wall-clock lag、diff rows、compile/bulk docs/s、
refresh-to-visible、重试量、Writer latency、恢复前后 Search completeness。

**通过标准**：

- 恢复后追到测试开始时声明的最终 desired target，遗漏和错 basis 为 0；
- 30 分钟断连期间 Canonical Writer 继续完成，不同步等待 OpenSearch；
- 增量追赶不执行双 Snapshot 全量 List，steady 热路径全仓扫描计数为 0；
- 追平时间、峰值 backlog 与资源作为该配置的容量结论报告。

**证据**：desired/basis 时间线、diff/bulk checkpoint、Writer 对照 histogram、恢复后的
全量 digest/抽样回读。

### KC-PERF-09 在线全量 rebuild

**目的**：验证新 generation rebuild、追赶和原子发布期间，旧 generation 继续服务，
Writer 与 Search 不发生错误耦合。

**前置**：S2 先校准，S3 通过后执行 S5；旧 active generation 可正常服务。

**负载**：从固定 commit BeginGeneration，分页 scan/compile/bulk；全程施加 steady 写和
KC-PERF-07 Search。分别在 bulk 中途、Publish 前和 Publish 后注入一次失败并重试。

**观测**：scan/compile/bulk 吞吐、checkpoint、临时磁盘、峰值 RSS、desired lag、旧/新
generation Search 延迟与错误、重复 bulk 工作量、publish/abort/cleanup 时长。

**通过标准**：

- S5 rebuild + catch-up ≤ 48 小时；
- 发布前旧 active 持续可用，失败的临时 generation 可回收；
- 发布原子，任何成功 Search 只使用一个 generation/basis；
- rebuild RSS 与 page/batch size 有界，Writer latency 不等待 rebuild；
- 旧 generation 的清理晚于 continuation/PIT 有效窗口。

**证据**：generation 生命周期 timeline、Search availability/completeness、checkpoint、
资源和临时容量、发布前后 basis 抽样。

### KC-PERF-10 历史老化与 command ledger

**目的**：量化真实 row change 历史增长对启动、当前/历史读取、diff、object log、commit
和幂等 ledger 的退化，并得到单 generation 的 `Hmax`。

**前置**：S2 和 S3 各自固定 current-state footprint；从同一基线依次生成 H0、H1、H2、
H3、H4 checkpoint。每个 commit 至少修改一个真实 unit/object manifest，禁止空 commit。

**负载**：每到一个 H 档执行同一 probe corpus：server startup、Head/GetRef/HasCommit、
current/historical Read/ReadMany、10,000 commits gap diff、Log limit 20/100、ledger
reserve/replay/conflict、一次真实 commit、backup/restore。

**观测**：各 probe P50/P95/P99、startup time/RSS、ledger bytes/entry 和 pending count、
Dolt reachable/working bytes、GC、backup/restore 时间与容量。

**通过标准**：

- H4 startup ≤ 60 秒且不加载全部 ledger；
- H4 current Read、commit 和 ledger point lookup 相对 H0 退化均 ≤ 25%；
- 普通 point/log/diff 路径无全历史扫描；
- 若 H4 失败，报告最高通过档 `Hmax`，推荐 rollover 不高于 `50% * Hmax`，不得将
  失败档宣传成能力。

**证据**：每个 H 档 checkpoint manifest、probe corpus digest、性能/容量斜率、backup/
restore 记录、Hmax 和推荐 rollover。

### KC-PERF-11 负载下 crash/retry

**目的**：测量关键失败窗口内的 fail-closed、恢复时间和重复工作量，而不是只在空闲系统
验证功能恢复。

**前置**：S2-H1；施加 KC-PERF-02 steady 写入和固定读/Search 负载。

**负载**：每个故障点独立 run，分别在 Reserve 后/Dolt 写前、SQL rows 写后/commit 前、
Dolt commit 后/ledger Complete 前、Receipt 后/desired target 前、OpenSearch bulk 中、
generation Publish 前后、source checkpoint 写前后执行 kill -9；重启后用同 command/event
重试并继续原负载 30 分钟。

**观测**：故障发现时间、服务恢复时间、backlog drain、ledger pending、重复 SQL/bulk
工作量、成功请求延迟、错误/partial 比例和最终 basis。

**通过标准**：

- partial Canonical、重复 revision、source cursor 越过失败 Receipt 均为 0；
- PENDING command 可证明完成或安全重放，不能静默遗留；
- OpenSearch 故障不阻塞 Canonical Writer，旧 active generation 保持可用；
- 恢复后最终 digest 与无故障对照 run 一致；恢复时间作为明确的 RTO 结果报告。

**证据**：每个 kill point 的 timeline、重启日志/trace、ledger 状态、对照 digest、恢复
期间的 availability/latency。

### KC-PERF-12 Generation rollover 与归档恢复

**目的**：在历史深度触及候选上限时，验证新 generation 建立、切换、旧仓 detach/restore
和性能回归是否满足长期运行要求。

**前置**：S3-H3 与 S3-H4 分别执行；输入同版本、同环境已批准的 Hmax 报告与只读
checkpoint，不依赖 KC-PERF-10 留下的可变运行状态。

**负载**：持续 steady 事件与读/Search；bootstrap 新 generation current state，追到
watermark，切换 WorkspaceDefinition，将旧 Repository archive/read-only 并从热 server
detach。随后 restore 旧 generation，以显式旧 pin 执行 Read/Log，再在新 active 上重复
KC-PERF-05 核心 probe。

**观测**：bootstrap/catch-up/cutover、短时错误与延迟、active/archived/backup bytes、detach/
restore duration、旧 pin probe、新 active 与 fresh-generation 基线差异。

**通过标准**：

- rollover 阈值不高于 `50% * Hmax`，并同时满足 bytes、backup、restore 的更早限制；
- 切换中 source event 不丢失，成功请求 basis 明确，普通应用读到 archived repo 为 0；
- restore 后显式旧 pin 的 Read/Log 成功；
- 新 active 的核心 probe 回到同规模 fresh-generation 基线的 25% 退化范围内；
- 输出 archive/restore 的 RPO、RTO 和五年容量区间。

**证据**：切换前后 Workspace pin、generation timeline、事件对账、容量清单、restore trace、
新旧 probe 对照和最终运营阈值。

## 5. 执行分级

| 级别 | 用例与档位 | 触发 |
| --- | --- | --- |
| PR | 不运行本清单；只运行功能/组件门禁和 S0 小 benchmark | 相关改动 |
| Nightly | KC-PERF-05/07，S1-H0 | 专用 runner |
| Weekly | KC-PERF-01/05/07/08，S2-H1 | Dolt/OpenSearch 集群 |
| Release candidate | KC-PERF-01–09/11，S3-H2/H3 | 容量环境 |
| Qualification | KC-PERF-01–12 的各自资格档，覆盖 S5 与 H4 | 首次上线或规模组件重大变化 |

Dolt major、layout major、OpenSearch mapping/shard policy、硬件或 durability 变化会使旧基线
失效，必须重新建立相应档位的 `BASELINE`，不能跨环境沿用绝对吞吐结论。

## 6. 结果目录与最小报告

```text
.data/data-warehouse/runs/scale/<run-id>/
├── manifest.json
├── model.json
├── timeline.jsonl
├── metrics/
├── traces/
├── sql/
├── correctness.json
├── capacity.json
└── report.json
```

`report.json` 除总体结果外，至少写入 `caseId`、档位、声明并发/速率、有效样本窗口、
每个 gate 的实测值和证据路径。`FAILED` 表示被测系统未过门槛；`INVALID` 表示环境或
证据不足，二者不得混用。原始 metrics、trace 和规模生成物不提交仓库。
