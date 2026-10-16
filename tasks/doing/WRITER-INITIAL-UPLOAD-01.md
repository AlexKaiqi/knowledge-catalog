# WRITER-INITIAL-UPLOAD-01 · 目录 bulk 与空仓首次上传快路

## 目标

已有 `writer commit --dir` 自动构造 ChangeSet；显式 `--bulk` 传到远端 Writer。
首次上传到可确认为空的固定基点时，CLI 和 Writer 都省掉逐对象旧值读取，保留 Schema、路径、并发和幂等约束。实际本机和 ttyd client 同步更新。

## 依据

- [对象写入](../../doc/design/object.md)：单仓 Writer、Schema、并发与幂等边界。
- [Snapshot](../../doc/design/snapshot.md)：固定 commit、目录读取和介质能力。
- [Writer 合同](../../object/writer/README.md)：目录输入与 bulk 条件提交。
- [质量闭环](../../doc/engineering/quality.md)：增量回归和门禁。

## 验收

- 远端目录输入在 bulk 开关、过滤未变化对象后仍携带正确选项；不支持的介质明确拒绝。
- head 仅对同一固定 commit 给出可靠空树信息；未知能力按原流程、读取失败拒绝，已有任意文件不视为空。
- 空仓首次目录提交没有逐对象 Resolve，Writer 没有逐对象旧基点文件读取；非空与显式历史 base 保留原差异语义。
- 无效 Schema、同批路径冲突、CAS 冲突仍被拒绝，批量介质调用、命令幂等保持；零变化目录重放也核对原请求，不绕过 bulk/ref/内容摘要冲突。
- 真实 lakeFS 通过已安装 client 的目录入口验证首次空仓 bulk/普通上传、后续增量、回读、Schema 原子拒绝、重放和并发 CAS；空仓旧值查询为零，介质传输有明确对照。
- 本机新终端默认命中更新 client，ttyd 持久镜像与实际二进制一致，连接配置与会话保留。
- 受影响完整回归与架构合同、`make check-docs`、`make quality` 全绿。业务大仓容量与耗时另行验收。

## 前置

无。

## 不做

不新增压缩包导入，不新增用户必填参数或 ChangeSet 文件，不自动拆批，不发布已整理的真实业务仓，不承诺未实测的大仓耗时。

## 并入

`DEBT-WRITER-DIR-BULK-01`：`IngestDesired` 未设置 BulkIngest，导致远端目录选项静默失效。
剩余验收包括实际远端 CLI 传输、未变化过滤、缺少介质能力时拒绝以及工程门禁。

## 进展

- 目录 bulk 三条回归已先失败再通过，包含实际 HTTP 提交及能力拒绝；专项 race 已通过。
- 空仓首次上传回归先失败：CLI 逐对象 Resolve；Writer 对 65 个待写对象产生 463 次旧基点文件读取。
- 证据目录：`.validation/runs/20261016-writer-initial-upload/`。

- 复核发现零变化目录仅按 command ID 返回旧回执，实际 Writer 夹具已复现 bulk 切换绕过摘要冲突；并入同一目录发布链路修复并补双入口合同。

- 用户要求暂停完整验收；完整场景在等共享搜索租约时已取消。实现、专项与质量门禁已有通过记录，完整验收保留待续，未宣称完成。

- 用户恢复验收，要求实际 client 更新；本轮补真实 lakeFS 的目录 bulk / 空仓快路专项，并核对执行的 client 与 Server 二进制。
- 补齐完整 APPLIED 元数据逐一缺 kind/repository/ref 的六个回归；旧实现 overlay 均红，当前普通与 race 均绿，无额外写入。
- 恢复轮证据：`.validation/runs/20261016-writer-upload-resumed/`；验收临时仓与状态独立，不触碰已整理业务仓。

- 真实 lakeFS 16 项通过：128 实例加 1 Schema 首次 bulk，Resolve=0、旧基点 GET=0、根目录探测=2、bulk POST=263、staging=0；非空增量、普通上传对照、回读、Schema 拒绝、重放和 CAS 均通过。最终临时容器、仓、state/cache 及两轮专属 S3 前缀均已复核清理。
- 本机 client 已正式安装到用户目录并调整新终端优先级；ttyd 仅重建 client 服务到唯一新镜像，实际 binary/metadata 一致，配置和会话指纹保留。
- 并行任务只改动仓内记录也会改变完整验收源码指纹；在独立工作树固定 `8b762331` 并加入本轮两份回归，1448 个 Go/module 输入与主工作区逐字节相同，完整 contracts 在此冻结候选执行，证据直接保存在主证据目录。
