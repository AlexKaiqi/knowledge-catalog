# WRITER-INITIAL-UPLOAD-01 · 目录 bulk 与空仓首次上传快路

## 目标

已有 `writer commit --dir` 自动构造 ChangeSet；显式 `--bulk` 传到远端 Writer。
首次上传到可确认为空的固定基点时，CLI 和 Writer 都省掉逐对象旧值读取，保留 Schema、路径、并发和幂等约束。

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
- 受影响完整回归与架构合同、`make check-docs`、`make quality` 全绿。真实部署与耗时另行验收。

## 前置

无。

## 不做

不新增压缩包导入，不新增用户必填参数或 ChangeSet 文件，不自动拆批，不发布真实仓，不承诺未实测的大仓耗时。

## 并入

`DEBT-WRITER-DIR-BULK-01`：`IngestDesired` 未设置 BulkIngest，导致远端目录选项静默失效。
剩余验收包括实际远端 CLI 传输、未变化过滤、缺少介质能力时拒绝以及工程门禁。

## 进展

- 目录 bulk 三条回归已先失败再通过，包含实际 HTTP 提交及能力拒绝；专项 race 已通过。
- 空仓首次上传回归先失败：CLI 逐对象 Resolve；Writer 对 65 个待写对象产生 463 次旧基点文件读取。
- 证据目录：`.validation/runs/20261016-writer-initial-upload/`。

- 复核发现零变化目录仅按 command ID 返回旧回执，实际 Writer 夹具已复现 bulk 切换绕过摘要冲突；并入同一目录发布链路修复并补双入口合同。
