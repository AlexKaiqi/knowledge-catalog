# DEBT-WRITER-UPLOAD-SCALE-01 · 大仓上传剩余线性往返

## 债

- 位置：`cli/desired.go` 的 `remoteCurrentDigests`；`invocation/verbs_write.go` 的 `CurrentDigests`；
  `object/writer/treecodec.go` 每对象 locator 文件。
- 信号：非空仓目录上传时，远端 CLI 对每个 Address 发一次 Resolve（32 并发），本地入口完全串行，
  服务端随后在 Writer 内再读一遍同一批对象。写入侧每个新对象至少 2 个文件（unit + locator）。
  2026-10-16 实测本机 lakeFS 1.86 + MinIO：lakeFS 自身吞吐约 2000–3400 文件/s（32–128 并发），
  修复后 Writer 端到端 1 万对象首传 7.7s、全量更新 7.3s，已贴近 lakeFS 自身上限，剩余主要是文件数。
- 判定：已修 Writer 内重复读、串行点读、平方级规划、未变文件重写与连接池
  （证据 `.validation/runs/20261016-upload-perf/`、`.validation/runs/20261016-write-perf/`）。
  剩余三项都要新公开形状或存储布局：批量 Resolve 端点、Writer 与 CLI 共用一份对照计划、
  分片或合并 locator。lakeFS import 零拷贝维持 LAKEFS-02 的否决（字节必须经 Writer）；
  Snapshot 层跨请求文件缓存受 CA-01 约束不做。
- 处置：先在 [对象写入](../../doc/design/object.md) 裁定 locator 是否可合并/分片及其并发冲突代价，
  再评估批量 Resolve（或固定 commit 分页列举携带摘要）替代逐对象预检。
- 触发条件：万级以上非空仓上传耗时验收不达标，或下一次修改目录发布链路 / locator 格式时。

## 目标

非空大仓上传的旧值对照与写入文件数不再与对象数线性相关，或有实测证明线性部分不构成瓶颈。

## 依据

- [对象写入](../../doc/design/object.md)：单仓 Writer、条件提交与幂等边界。
- [Snapshot](../../doc/design/snapshot.md)：固定 commit、目录读取和介质能力。
- [工程质量闭环](../../doc/engineering/quality.md)：增量不欠与质量问题三个出口。

## 验收

- 万级对象非空仓上传在真实 lakeFS 上给出请求数与耗时的前后对照。
- Schema、路径冲突、CAS、幂等与零变化重放语义不变，对应回归通过。
- 受影响完整回归、`make check-docs`、`make quality` 全绿。

## 前置

无。

## 不做

不自动拆批，不新增用户必填参数，不改变 Canonical 正文格式。
