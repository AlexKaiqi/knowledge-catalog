# DEBT-WRITER-UPLOAD-SCALE-01 · 大仓上传剩余线性往返

## 债

- 位置：`cli/desired.go` 的 `remoteCurrentDigests`；`invocation/verbs_write.go` 的 `CurrentDigests`；
  `object/writer/treecodec.go` 每对象 locator 写入；`snapshot/internal/lakefsimpl` 的逐文件暂存/上传。
- 信号：非空仓目录上传时，远端 CLI 对每个 Address 发一次 Resolve（16 并发），本地入口完全串行，
  且服务端随后在 Writer 内再读一遍同一批对象；写入侧每对象至少 2 个文件、每文件 1 次（bulk）或
  3 次（presign 暂存）请求。请求数仍与对象数线性相关，只是从串行降为有界并发。
- 判定：本轮已修 Writer 单命令内重复读、串行点读与 lakeFS 连接池过小
  （证据 `.validation/runs/20261016-upload-perf/`）；剩余项需要新增批量形状或改变存储布局，
  属于公开形状或格式决策，不在本轮顺手扩张。
- 处置：评估按固定 commit 分页列举（携带 checksum）一次求差，替代逐对象 Resolve；评估 Writer
  对照与 CLI 预检共用一份计划；评估 lakeFS import/批量暂存替代逐文件 POST；
  评估分片 locator 以减少每对象文件数。每项先给出设计篇依据再实施。
- 触发条件：业务大仓（万级对象）上传耗时验收不达标，或下一次修改目录发布链路 / lakeFS 写路径时。

## 目标

非空大仓上传的旧值对照与写入请求数不再与对象数线性相关，或有实测证明线性部分不构成瓶颈。

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
