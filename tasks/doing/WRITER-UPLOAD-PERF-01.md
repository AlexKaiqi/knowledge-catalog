# WRITER-UPLOAD-PERF-01 · 非空仓上传的重复读与串行点读

## 目标

非空基点的目录上传中，Writer 一次命令内每个 `(commit, path)` 至多读 authority 一次，
独立点读以有界并发进行；lakeFS adapter 的并发请求复用连接。Schema、路径准入、CAS、
幂等与错误语义不变。

## 依据

- [对象写入](../../doc/design/object.md)：单仓 Writer、Schema 与条件提交边界。
- [Snapshot](../../doc/design/snapshot.md)：固定 commit 不可变、介质能力。
- [Writer 合同](../../object/writer/README.md)：文件型 codec 只读涉及对象的 locator 与 unit。
- [质量闭环](../../doc/engineering/quality.md)：增量回归和门禁。

## 验收

- 非空仓混合更新/新增 64 对象时无重复基点读取；点读峰值并发大于 1；路径占用拒绝保留。
- lakeFS 32 并发 × 6 轮请求的新建连接数不超过并发度的 1.25 倍。
- 受影响普通与 race 回归、`make test`、`make check-docs`、`make quality` 全绿。

## 前置

无。

## 不做

不新增公开形状或批量端点，不改 Canonical 正文与 locator 格式，不改 presign 数据面；
剩余线性往返登记为 `DEBT-WRITER-UPLOAD-SCALE-01`。

## 进展

- 先红：Writer 64 对象非空更新 334 次读取、135 个不同坐标、101 个坐标重复；点读峰值并发 1、
  耗时 0.81s；lakeFS 32 并发 × 6 轮新建 182 条连接（默认每主机空闲连接 2）。
- 实现：`object/writer/basis_cache.go` 单命令不可变基点读视图（缺失结果复用，传输失败不缓存）
  与 32 路有界预取；`readChangedObjects` 两阶段预取 locator 与 unit，路径准入预取目标路径，
  消费仍串行有序；lakeFS API 与直连客户端每主机空闲连接按 staging 并发度放大（至少 64）。
- 后绿：三条 Writer 回归与连接复用回归通过，并发用例 0.03s。
- 计数夹具（writer 两个、reader 一个）改为加锁计数以适配并发读取，断言未改。
- object/snapshot/invocation 与 liveaccess/discovery/home/server/internal race 无 data race；
  `make check-docs`、`make quality` 通过。CLI 及部分 home/server/liveaccess 用例因未提供
  OpenSearch/索引插件环境失败（`KC_TEST_OPENSEARCH_URL`、`Dataset index plugin is not configured`），
  未计为通过。
- `make test` 未执行：执行时另一条线正占用共享 OpenSearch 跑 `make test-contracts`。
- 证据：`.validation/runs/20261016-upload-perf/`。真实 lakeFS 与业务大仓耗时未测。

### 第二轮：大仓规划线性化与未变文件不重写

- 先红（真实 lakeFS 1.86 + MinIO，容器网络内，1 万对象）：首传 16.6s、全量更新 21.0s；
  Writer CPU 18.6s，热点为 `CanonicalDigest` 整体拼串、`ApplyUnits` 删除实体时全表扫兄弟、
  `finalObjectUnits` 每对象全表过滤（均为平方级）；只改值的更新仍重发全部文件。
  同环境 lakeFS 自身 apply 约 2000–3400 文件/s。
- 实现：`kernel/digest.go` 流式写入哈希（字节形式与旧实现逐字节一致）；`object/units.go`
  按对象 ID 建 unit 索引；`object/writer/schema_batch.go` 惰性按对象分组最终 unit；
  `object/writer/basis_cache.go` `dropUnchangedWrites` 剔除与已读基点字节相同的写入
  （全部未变保留原集合）；`cli/desired.go` 预检并发 16→32 与服务端对齐。
- 后绿：`TestStreamedCanonicalFormMatchesLegacy`、`TestCanonicalDigestLargeArrayIsLinear`、
  `TestApplyUnitsScalesLinearly`、`TestApplyUnitsRemoveEntityDropsAllSiblingsOnly`、
  `TestUpdateSkipsByteIdenticalFiles`、`TestWriterUpdateScalesLinearly` 通过；1 万对象首传 7.7s、
  更新 7.3s，Writer CPU 1.5s；`KC_LAKEFS_STAGE_CONCURRENCY=64` 再快约 20%，默认值不变。
- 门禁：基于 HEAD 仅含本轮文件的独立 worktree 中 `make check-docs`、`make quality`、`make test` 全绿；
  主工作区 `make check-docs` 通过。
- 未做：import 零拷贝（LAKEFS-02 否决）、跨请求缓存（CA-01）、presign 数据面（I-53）；
  批量 Resolve、Writer/CLI 共用对照计划、locator 合并登记在 `DEBT-WRITER-UPLOAD-SCALE-01`。
- 证据：`.validation/runs/20261016-write-perf/`。
