# Knowledge Catalog 规模压测设计

日期：2026-08-27
状态：待实现测试设计（与 `SCALE_ARCHITECTURE.md` 的 native Dolt 方案一致）

本文只定义压测模型、执行阶段、证据和验收门槛。实现改动见 [`SCALE_ARCHITECTURE.md`](SCALE_ARCHITECTURE.md)。
数仓知识提供方的独立、可逐条执行用例见
[`../.data/data-warehouse/scale/CASES.md`](../.data/data-warehouse/scale/CASES.md)；它们不属于
`features/*.feature` 的功能验收。

---

## 1. 压测要回答的问题

1. native Dolt 在 1,000,000 tables 对应的真实 objects/units/endpoints 下能否完成 bootstrap、点读、分页和关系查询？
2. 每天 10,000 个 source changes、每个变化立即 commit 时，稳态和 `100x` burst 是否满足时效？
3. command ledger、进程启动、HEAD、点读、diff 和 object log 在 1m/5m/10m/20m commit 后如何退化？
4. OpenSearch 能否在不阻塞 Writer 的前提下追赶、rebuild 和切 generation？
5. active Repository generation 应在什么 commit/bytes/restore-time 阈值切换？
6. 旧 generation 归档、detach、restore 后，旧 `{repository, commit}` 是否仍可读取？
7. 是否存在任何普通热路径退化为全仓扫描或一次性全量内存物化？

测试不以“命令成功”代替容量结论，也不通过空 commit、极小 JSON 或关闭历史来制造漂亮数字。

---

## 2. 数据模型

### 2.1 当前模型基线

当前 `.data/data-warehouse/connector/domain.py` 对每条 containment edge 生成一个 Relation object。每表 `C` 列时：

```text
objects/table  = 1 table + C columns + 1 schema-table relation + C table-column relations
               = 2C + 2
units/table    = 2 table aspects + C column aspects + C + 1 relations
               = 2C + 3
endpoints/table = 2(C + 1)
```

`C=30` 时是 62 objects、63 units、62 endpoints。该模型只用于 current-baseline slope 和迁移差分，不作为新 target 的容量模型。

### 2.2 目标 grouped-relation 模型

新模型每表保留两个 Relation objects：schema-table 与 table-columns。每表 `C` 列时：

```text
objects/table   = 1 table + C columns + 2 relations = C + 3
units/table     = 2 table aspects + C column aspects + 2 relations = C + 4
endpoints/table = 2 + (1 + C) = C + 3
```

Schema、Database、Platform、DataJob 和 semantic objects 另行按比例加入，不能省略，但其数量相对 Table/Column 为低阶项。

### 2.3 规模档位

| 档位 | Tables | Columns/table | 主体 objects | 主体 units | Relation endpoints | 用途 |
|---|---:|---:|---:|---:|---:|---|
| S0 | 1,000 | 10 | 13,000 | 14,000 | 13,000 | 本地功能 smoke |
| S1 | 10,000 | 10 | 130,000 | 140,000 | 130,000 | 当前实现 slope / PR benchmark |
| S2 | 100,000 | 30 | 3,300,000 | 3,400,000 | 3,300,000 | pilot 与索引校准 |
| S3 | 1,000,000 | 30 | 33,000,000 | 34,000,000 | 33,000,000 | 正式 target |
| S4 | 1,000,000 | 50 | 53,000,000 | 54,000,000 | 53,000,000 | wide-table stress |
| S5 | 2,000,000 | 50 | 106,000,000 | 108,000,000 | 106,000,000 | 过亿对象 qualification 基线 |

另设 R1 高基数关系档：10,000 tables、每表 500 columns，用于验证 256-endpoint 上限、稳定 bucket 分片、单列变化写放大和 Relation candidate paging；它不与 S4 的百万表总量同时运行。

每个档位额外生成：

- 1 platform / 100 database / 10,000 schema；
- DataJob 数为 tables 的 1%；
- 每个 DataJob 2 aspects 和一组 2–20 个 input/output endpoints；
- Schema object 集覆盖所有实际 Aspect；
- semantic objects 默认为 physical table 数的 1%，另设 10% stress；
- 5% columns 带更长 comment/description，1% objects 带 Binding declaration；
- value 大小使用真实夹具分布采样，不固定成几个字节。

### 2.4 可重复生成

生成器必须：

- 固定 seed；
- 流式产生 tables/families，不在内存构造全数据集；
- object_id/source key 与夹具相同规则；
- 输出每类数量、canonical bytes、value-size histogram 和全局 digest accumulator；
- 支持从 checkpoint 继续；
- 可单独生成 target current state 和历史 change stream。

---

## 3. Commit 历史档位

每天 10,000 commit 的换算：

| 档位 | Commits | 等效持续时间 | 目的 |
|---|---:|---:|---|
| H0 | 100,000 | 10 天 | 快速退化检查 |
| H1 | 1,000,000 | 100 天 | 首个历史门槛 |
| H2 | 5,000,000 | 500 天，约 1.37 年 | 中期运行 |
| H3 | 10,000,000 | 约 2.74 年 | generation 候选安全线 |
| H4 | 20,000,000 | 约 5.48 年 | 五年资格验证 |

H4 是目标资格测试，不表示生产一定等到 20m 才切换。生产 rollover 上限为最大通过档位 `Hmax` 的 50% 或更低，并同时服从 bytes、备份和恢复门槛。

每个历史 commit 至少修改一个真实 unit/object manifest。禁止用 `--allow-empty` 代替实际历史，因为空 commit 不产生相同的 row diff、Prolly-tree 和存储成本。

---

## 4. 变更负载

### 4.1 逻辑事件混合

| 事件 | 比例 | 典型知识变化 |
|---|---:|---|
| table property/comment/type 变化 | 35% | table properties unit + manifest |
| column type/nullability/comment 变化 | 30% | 1–3 column units + manifests |
| add/drop column | 15% | column unit、table schema、grouped contains relation/endpoints |
| create/drop table | 8% | 整个 table family add/remove |
| table/column rename | 4% | old/new source keys，bounded family rewrite |
| DataJob/lineage 变化 | 5% | job aspects + grouped lineage relation |
| semantic publish | 3% | semantic repo 独立 commit |

每次 source event 必须带 event id、source revision/LSN、family key 和发生时间。Collector 只能点拉受影响 family。

### 4.2 到达分布

- steady：10,000/day 等效速率；
- diurnal：峰值为平均的 10x；
- burst：平均的 100x，即约 11.6 events/s，持续 30 分钟；
- shock：1,000 events 在 10 秒到达，验证 backlog 和零等待 coalescing；
- duplicate：5%；
- out-of-order：2%；
- delete/rename 的 old-key 信号缺失：0.1%，用于 gap recovery。

低流量事件不得等待 batching timer。记录：

```text
source changed_at
event received_at
targeted pull complete_at
preview complete_at
Dolt commit_at
receipt_at
projection visible_at
```

### 4.3 单 commit 与合并 commit

默认一个 source transaction/逻辑表变化一个 commit。只有队列已经存在多条已到达消息时才测试零等待合并，分别报告：

- events/commit；
- 每个 event 的最老/最新等待时间；
- event IDs 是否完整进入 commit/observability evidence；
- 去重后是否产生额外 knowledge revision。

不能用固定 1s/5s/1min 窗口通过吞吐门槛。

---

## 5. 读与消费负载

### 5.1 Canonical mix

| 操作 | 比例 | 形状 |
|---|---:|---|
| READ object | 40% | Table/Column/Metric，1–6 units |
| READ address | 15% | 精确 Aspect/member |
| ReadMany | 15% | 10/50/100/500 IDs 四档 |
| Resolve | 10% | 已存在、已删除、不存在各占比例 |
| Relation retrieval page | 10% | table/column/job endpoint，page 100 |
| LIST page | 5% | page 100/500，连续翻 10 页 |
| Schema/AccessSpec | 3% | 全 Schema 与单 Schema |
| LOG/DIFF/Provenance | 2% | limit 20/100，旧 commit 抽样 |

冷热分布：70% 请求命中 1% hot objects，25% 均匀随机，5% 显式历史 commit。必须分别报告 cache hit/miss，不能只给混合值。

### 5.2 Workspace

至少包含 physical + semantic 两个 Repository，测试：

- Workspace list continuation 跨 member；
- 同一 object_id 在不同 member 的联邦读；
- 每个命令只解析一次 pin；
- continuation 与不同 pin/query 混用必须失败；
- archived generation 未授权时不参与普通 Workspace。

### 5.3 无界接口防护

scale profile 必须证明：

- Snapshot scan 只在 projection rebuild/export 验收中分页执行；不存在 CLI/HTTP Knowledge LIST；
- page size 超上限被 clamp 或拒绝；
- Relations 同样分页；
- checkout/export 通过 page iterator，RSS 不随总 objects 线性上升；
- `knowledge.Repository`、Serving 和 CLI 不再存在公开无界 `List()`；测试 helper 也只能循环消费 pages。

---

## 6. Projection 与 Search 负载

### 6.1 可索引文档量

默认 OpenSearch 只索引有 AccessHints 的 entity/aspect assembled document。纯 containment Relation 不入全文索引，native endpoint locator 负责关系查询。

S5 预期可检索主体约为 Table + Column，即 102,000,000 docs，再加 Job 与 semantic docs。压测必须记录实际 eligible docs，不能把 objects 数直接当 docs 数。

### 6.2 Rebuild

在固定 commit 上：

1. BeginGeneration；
2. Repository page read；
3. compile page；
4. OpenSearch bulk；
5. bootstrap 期间继续接收新 commit，记录 desired target；
6. 初始遍历后追赶差异；
7. Publish；
8. 旧 generation 延迟清理。

注入中途失败，验证旧 active index 仍服务、临时 generation 可回收、重试不全量重复已确认 page（若 provider 支持 checkpoint）。

### 6.3 增量

覆盖：

- 1 object/commit；
- 1 table family/commit；
- from→to 跨 1、100、10,000 commits；
- Projection 落后 1h/24h 后一次求 net object diff；
- Schema AccessDigest 变化触发新 generation；
- OpenSearch 断开 30min 后恢复。

每次候选必须携带 basis，回读必须使用同一 commit。

### 6.4 Search mix

- MATCH：AllTerms / AnyTerms / Phrase；
- typed EQ/IN/NEQ/EXISTS/MISSING；
- number/time range；
- string prefix；
- sort + continuation；
- relation lookup 走 native 与 OpenSearch compatibility lane 的结果差分。

---

## 7. 压测阶段

### P0. 正确性与差分

- Gitea 与 native Dolt 执行同一随机 ChangeSet 序列；
- 比较 Read/ReadAddress/Resolve/Log/Diff/Provenance；Relation 由独立 exact-basis Retriever conformance 覆盖；
- 校验 precondition、entity remove、path move、Schema/ValueSource 继承；
- manifest/unit/endpoint 全量 invariant check；
- grouped relation 与旧 edge relation 的 adjacency/role 语义差分，并验证 R1 endpoint 分片；
- crash/retry 前不得进入性能阶段。

### P1. 当前实现 slope

只跑 S0/S1，证明并量化本次审计，不把旧实现硬推到百万表：

- Writer 单 PUT 随 total units 的斜率；
- Dolt ReadFile 随 `kc_files` rows 的斜率；
- Reader point read 与 Relations 的斜率；
- command ledger 第 N 次提交的 bytes rewritten 和 startup RSS；
- index rebuild RSS。

P1 结果是改造依据，不是上线候选。

### P2. Native bootstrap

依次跑 S1、S2、S3：

- candidate ref keyset import；
- 500/1,000/5,000 tables per commit 对比；
- 并发 source page fetch + 单 writer；
- bootstrap event buffer/catch-up；
- Merge 发布；
- 冷/热 point read、page、relations；
- physical bytes 与 rows/table。

S4 只在 S3 通过后运行；S5 是过亿对象资格档，不得用 S3/S4 外推替代。

### P3. Steady state

- S5 current state（S3 可作为 release-candidate 预跑）；
- 连续至少 24h 等效 steady load；
- 每 event 立即 commit；
- 同时施加 Canonical read mix；
- OpenSearch controller 正常追赶；
- 记录 CPU、RSS、disk write、commit latency、event-to-visible。

### P4. Burst/backlog

- 10x diurnal 2h；
- 100x burst 30min；
- shock 1,000/10s；
- 比较严格单-event commit与零等待 queue coalescing；
- burst 停止后测 drain time；
- 低流量恢复后第一条事件不得等待合并窗口。

### P5. History aging

在 S2 和 S3 中选择固定 current-state footprint，持续产生真实 row changes到 H0/H1/H2/H3/H4。每档执行相同 probe：

- process/server startup；
- Head/GetRef/HasCommit；
- current 和 historical Read/ReadMany；
- from→to object diff；
- object Log limit 20/100；
- command ledger reserve/replay/conflict；
- backup/restore；
- Dolt storage bytes 与 GC（只统计 unreachable 回收，不宣称删除已达历史）。

每档保存 checkpoint，可跨多次专用任务继续，不要求一次进程跑完 20m。

### P6. Projection recovery

- S2 full rebuild 后再跑 S3，最终必须跑 S5；
- rebuild 中持续 commit；
- OpenSearch kill/restart；
- 丢失内存 notification；
- controller 重启从 basis/desired target 恢复；
- new generation publish 前后并发 Search；
- 校验无 Writer latency 耦合。

### P7. Failure injection

在以下边界 kill -9：

1. command Reserve 后、Dolt 写前；
2. SQL rows 写入后、DOLT_COMMIT 前；
3. DOLT_COMMIT 后、ledger Complete 前；
4. Receipt 后、projection desired target 前；
5. OpenSearch bulk 中；
6. generation Publish 前后；
7. source checkpoint 写前后。

验证无 partial canonical、无重复 revision、可证明恢复或明确 fail closed。

### P8. Generation/Archive drill

- 在 H3/H4 仓上创建新 generation candidate；
- bootstrap current state，不复制旧 commit history；
- watermark catch-up；
- WorkspaceDefinition 切换；
- old repo archive/read-only；
- old database 从热 server detach；
- 普通应用访问被拒绝；
- restore 后显式旧 pin Read/Log 成功；
- 新 active 的 latency 回到 fresh-generation 基线。

---

## 8. 环境

### 8.1 版本固定

证据必须记录：

- KC git revision；
- Go version；
- Dolt exact version/image digest；
- OpenSearch exact version/image digest；
- OS/kernel/filesystem；
- 配置、schema/layout version 和 generator seed。

禁止用 `latest`。

### 8.2 Dolt 参考档

Pilot 最低参考：

- 16 physical vCPU；
- 64 GiB RAM；
- 本地 NVMe，容量至少为预测 target bytes 的 3 倍；
- 独立 SQL Server；
- Writer 与 load generator 分机/分容器；
- 单 Repository active-writer lease。

S5/H4 资格测试建议从 64 vCPU、256 GiB RAM 和企业 NVMe 起步，但最终结论必须报告硬件，不把硬件差异藏在统一门槛后。

### 8.3 OpenSearch

S2 用 3 data nodes 校准：

- 每 node 16 vCPU、64 GiB RAM；
- heap 不超过该 node RAM 的合理比例；
- replica、shard、refresh interval 明确记录。

S5 不预设固定节点数。根据 S2/S3 实测 `primary bytes/doc`、segment/shard 开销和查询吞吐计算：

```text
required_primary_bytes = eligible_docs * measured_primary_bytes_per_doc
required_disk = primary_bytes * (1 + replicas) / target_disk_utilization
```

目标磁盘利用率不超过 60%，并留出 rebuild 时新旧 generation 共存空间。

### 8.4 网络与时间

- 所有机器 NTP 同步；
- 分别报告同机与跨机 RTT；
- event-to-visible 使用同一时间基准；
- load generator 自身 CPU <70%，否则结果无效。

---

## 9. 指标

### 9.1 Collector/Writer

- event receive、targeted pull、preview、queue、commit、receipt latency；
- source rows fetched/event，必须与 family size 有界；
- events/commit、operations/commit、rows mutated/commit；
- NON_FAST_FORWARD、duplicate、out-of-order、gap；
- active-writer lease wait；
- SQL transaction 与 DOLT_COMMIT latency；
- ledger Reserve/Lookup/Complete latency、bytes/entry、pending count；
- process startup time/RSS。

### 9.2 Reader

- Read/Address/ReadMany P50/P95/P99；
- rows scanned/returned；
- Relation retrieval latency、continuation size；ScanSnapshotPage 仅在维护基准中统计；
- Schema list 与 AccessSpec compile/cache；
- current vs historical commit latency；
- object Log/diff latency；
- cache hit/miss 分开。

### 9.3 Projection/Search

- desired target、basis、commit lag 和 wall-clock lag；
- object diff rows；
- compile/bulk docs/s、bytes/s；
- rebuild page checkpoint、RSS、temporary disk；
- refresh-to-visible latency；
- search P50/P95/P99、candidate/hydrated ratio；
- generation publish/abort/cleanup。

### 9.4 Capacity/Operations

- Dolt current rows、commits、reachable bytes、working disk、backup bytes；
- bytes/table、bytes/unit、bytes/endpoint、bytes/real change、bytes/commit；
- OpenSearch primary/replica bytes/doc；
- backup、restore、archive、detach、reattach duration；
- generation bootstrap/catch-up/cutover duration；
- CPU、RSS、GC pause、disk IOPS/latency、network。

Repository/Object/Commit IDs 不作为无界 metric labels；进入 trace/log sample。

---

## 10. 验收门槛

以下为首版 gate，硬件和版本必须随结果保存。

### 10.1 正确性硬门槛

| 指标 | 门槛 |
|---|---:|
| 丢失 source event | 0 |
| duplicate event 产生额外 revision | 0 |
| partial units/manifest/endpoints commit | 0 |
| 错 basis Canonical 回读 | 0 |
| hash collision 静默覆盖 | 0 |
| continuation 跨 pin/query 复用成功 | 0 |
| 普通应用读到 archived repo | 0 |
| Writer 等待 OpenSearch 完成 | 0 |

### 10.2 时效与吞吐

| 指标 | 门槛 |
|---|---:|
| steady event received -> Receipt P99 | ≤ 2 s |
| steady event received -> Search visible P99 | ≤ 5 s |
| 100x burst sustained intake | ≥ 12 events/s，30 min |
| 100x burst 后 backlog drain | ≤ 10 min |
| 低流量第一条事件 batching wait | 0 |
| targeted source pull rows | `O(changed family)`，禁止随全仓增长 |

### 10.3 Canonical 读

| 指标 | 门槛 |
|---|---:|
| current Read P99 | ≤ 100 ms |
| historical Read P99 | ≤ 250 ms |
| ReadMany(100) P99 | ≤ 500 ms |
| Relation retrieval page(100) P99 | ≤ 250 ms |
| Schema list P99 | ≤ 250 ms |
| object Log(limit=20) P99 | ≤ 500 ms |
| from→to changed objects（10k commits gap）P99 | ≤ 1 s |

### 10.4 启动、内存和历史

| 指标 | 门槛 |
|---|---:|
| KC service startup at H4 | ≤ 60 s，且不加载全部 ledger |
| steady RSS | 24h 内无随 object/commit 总数线性爬升 |
| checkout/rebuild RSS | 与 page/batch size 有界 |
| H4 current Read/commit latency 对 H0 退化 | ≤ 25% |
| H4 ledger point lookup 对 H0 退化 | ≤ 25% |
| S5 bootstrap candidate + catch-up | ≤ 48 h |
| S5 OpenSearch rebuild + catch-up | ≤ 48 h |

若 H4 未通过，记录最高通过档位 Hmax，并用 `≤50% Hmax` 作为 generation rollover 上限后重新执行 P8。只要 P8 通过且旧历史可恢复，系统仍可满足长期总历史保留；不得把失败档位宣传成单仓能力。

### 10.5 扫描禁令

steady hot path 以下计数必须为 0：

- Writer `ListFiles/List` 全仓；
- Read/Resolve/Schema/Relations 全仓；
- 每 source event 的 `describe_all_columns`；
- Projection 增量的双 Snapshot List；
- command ledger 全文件 rewrite/load；
- HTTP 一次序列化全 Repository。

通过 SQL trace、provider counters 和 Go span 三者交叉证明，不只依赖代码阅读。

---

## 11. 存储与五年容量报告

每个档位至少报告：

```text
Dolt bytes after bootstrap
Dolt bytes after N real commits
reachable vs working/unreachable bytes
bytes/object, bytes/unit, bytes/endpoint
incremental bytes/commit by event type
command ledger bytes/entry
backup compressed bytes and duration
OpenSearch primary bytes/doc and replica bytes
temporary bytes during rebuild/generation migration
```

五年估算不能只做线性总和，必须给区间：

```text
current_state_bytes
+ 18.25m * measured incremental bytes/change
+ command ledger
+ active OpenSearch generations
+ archived Repository generations
+ backup copies
+ safety/headroom
```

分别提供 P50/P95 event size 斜率，并说明 Dolt chunk dedup、grouped relation rewrite 和 value 大小对结果的影响。

---

## 12. 测试落点

### 12.1 仓库根

通用、不含数仓业务的测试放现有 package `_test.go`：

- `snapshot/commandlog/` keyed-store conformance、crash recovery；
- `knowledge/writer/` native/tree differential write；
- `knowledge/reader/` page/continuation/native capability；
- `knowledge/dolt/` SQL schema、point read、diff/history、failure；
- `index/` streaming rebuild、desired-target recovery；
- `retrieval/opensearch/` generation/bulk/search；
- `internal/arch/` 新依赖图守卫；
- `cli/` page HTTP shape 与禁止全量 fallback。

小规模性能回归可放 Go benchmark，但不得让普通 `make test` 启动 S2–S4/H1–H4。

### 12.2 数仓夹具

业务模型和 source event 压测生成物只放受忽略的 suite 子目录：

```text
.data/data-warehouse/scale/
├── generator/
├── events/
├── checkpoint/
├── load/
├── profiles/
└── README.md
```

目标结果放：

```text
.data/data-warehouse/runs/scale/<run-id>/
```

`.data/data-warehouse/scale/` 只跟踪 generator 与说明；events/checkpoint/load/profiles 和 `runs/` 不提交。稳定后的大规模 harness 可迁出独立 integration repository。仓库根不增加数仓实体、Collector runtime 或新 Write Surface。

### 12.3 执行分级

| 级别 | 档位 | 触发 |
|---|---|---|
| PR | P0 + S0 | 每次相关改动 |
| Nightly | S1 + H0 | 专用 runner |
| Weekly | S2 + H1 | Dolt/OpenSearch 集群 |
| Release candidate | S3 + H2/H3 | 容量环境 |
| Qualification | S5 + H4 + P8 | 首次上线、Dolt major、layout major、OpenSearch mapping/shard policy 或硬件变更 |

---

## 13. 证据格式

每次 run 必须生成：

```text
manifest.json          versions, hardware, config, seed, git revision
model.json             counts and byte histograms
timeline.jsonl         phase and fault events
metrics/               Prometheus snapshot / raw samples
traces/                sampled slow/error traces
sql/                   normalized query stats and scan counters
correctness.json       invariants and differential digests
capacity.json          Dolt/OpenSearch/ledger/backup bytes
report.json            gates, pass/fail, Hmax, recommended rollover
```

`report.json` 至少包含：

```json
{
  "profile": "S3-H4",
  "result": "PASSED",
  "highestPassingCommitTier": 20000000,
  "recommendedGenerationRollover": 10000000,
  "gates": [],
  "artifacts": []
}
```

原始 metrics/traces 不提交到仓库；结论必须能由 manifest + evidence 复算。

---

## 14. 停止条件

出现以下任一情况立即停止加压，保存证据并先修正确性：

- Canonical/manifest/endpoint 不一致；
- duplicate revision、source cursor 越过失败 Receipt；
- wrong-basis hydration；
- command PENDING 无法证明恢复；
- Repository ref/CAS 被并发写破坏；
- OpenSearch publish 让旧 active index 不可用；
- OOM、磁盘 >85%、持续 I/O error；
- load generator 饱和导致假瓶颈；
- steady path 出现任何全仓扫描计数。

性能不达标但正确性仍成立时，可以完成当前测量点后停止；不得自动改变模型、历史开关、replica 或 durability 再把结果合并报告。

---

## 15. 最终决策输出

资格测试后必须明确输出，而不是只给图表：

1. S5 是否通过；若否，瓶颈在 Dolt current rows、Writer commit、Reader、ledger 还是 OpenSearch。
2. 单 generation 的 `Hmax`。
3. 生产 rollover commit/bytes/age 三个阈值，取最早者。
4. 五年 active + archived + backup 总容量区间。
5. steady/burst 的事件到 Receipt、到 Search 时延。
6. archive detach/restore 的 RPO/RTO 与允许访问面。
7. 是否继续采用 native Dolt。

若 S5 current-state 或低于一年等效历史仍失败，说明当前整体 scale profile 不合格；必须定位 Canonical 或 Projection 瓶颈，Canonical 失败时启动新的 MVCC Snapshot adapter 设计，不能回退到 `kc_files + 伴随表`。若只是 H3/H4 失败而 P8 通过，则保留 Dolt 并降低 generation rollover 上限。
