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

### 第三轮：真实仓包实测、归档输入、连接池与批量通路重试

- 数据：`.data/realcase/prepared-v3/` 两个真实仓包，`base_mini.tar.gz`（87078 文件）与 `business.tar.gz`
  （45634 文件）；本机 lakeFS 1.86 + MinIO，服务端在容器内、客户端在宿主。
- 先红：目录输入逐个读小文件，客户端读目录 24s；Go 默认每主机仅 2 条空闲连接，预检并发拉高后
  宿主临时端口耗尽报 `EOF`；`base_mini` 首传 669s。
- 实现：`object/writer/ingest_source.go` 支持 `.tar/.tar.gz/.tgz[//子目录]` 输入与有界并发解析
  （读目录 24s→1.3s）；`internal/httpjson/transport.go` 共用连接池，`client`、`cli/remote_session.go`
  与 lakeFS adapter 共用；`--bulk` 的 `uploadObject` 对 5xx、429 与传输错误做 6 次退避重试并保留 lakeFS 响应体。
- 后绿：`base_mini` 首传默认路径 669s→HEAD 107s→76s；`--bulk` 连续 3 次 97–110s 成功；
  写后零差异；回归 `TestUploadObjectRetriesTransientServerErrors`、`TestUploadObjectGivesUpAfterBoundedAttempts`、
  `TestUploadObjectDoesNotRetryClientErrors`（先把重试次数改 1 确认变红再还原）。
- 根因（`--bulk` 此前在完整仓稳定 500）：服务端日志为 lakeFS 到 MinIO 的 `PutObject` 报
  `dial tcp ...:9000: cannot assign requested address`，即 lakeFS 容器出站临时端口耗尽；
  与 KC 逻辑、文件大小、路径长度、规模无关。Go 客户端并发 32 的瞬时速率高于 Python 探针故先触发。
  随后客户端因回执恢复把"明确未应用"也当成结果未知，空等 `KC_COMMIT_RECEIPT_WAIT`（默认 10 分钟），
  使失败总耗时约 11 分钟。
- 结论：`--bulk` 在本机栈上不比默认路径更快（97–110s 对 76–117s，机器有他线负载），默认路径
  仍是首选；`--bulk` 保留给无法使用 presign 的介质。
- 业务包 `business.tar.gz` 含 43 个 readme 方面，而协议规定仓根 `README.md` 唯一，原包直传
  报 `OBJECT_ID_CONFLICT`；仅在 /tmp 副本去掉 42 个非根 README 后测速（65.6s），未改协议和数据。
  数据如何处理待用户决定。
- 无变化重传需沿用原命令号：新命令号得到 `unknown command-id` 是目录重放合同
  （`cli/desired_replay.go`），不是缺陷。
