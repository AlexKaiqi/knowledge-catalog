# DEBT-WRITER-DIR-BULK-01 · 远端目录提交漏传 bulk 选项

## 债

- 位置：`cli/remote_management.go:76` 的 `runRemoteDesiredCommit`，`cli/desired.go:117` 的
  `prepareRemoteDesiredIngest`，`invocation/verbs_write.go:342` 的 `IngestDesired`。
- 信号：帮助公开 `writer commit --dir ... --bulk`，但远端目录分支经 IngestDesired 创建 ChangeSet 后
  直接提交，整个分支没有设置 BulkIngest；writer.Ingest 初始化值为 false。
  对照 `--changeset` 分支在 `cli/remote.go:115` 显式赋值，本地目录提交在 `invocation/verbs_write.go:185` 也有赋值。
- 判定：公开选项在业务远端目录路径静默失效，用户请求的批量传输未送达 Writer。当前为源码路径核对，未执行实际远端发布。
- 处置：补齐共享目录构造流程的选项传递，增加经过真实远端 CLI 请求构造的回归；不改介质 bulk 实现或发布语义。
- 触发条件：准备以远端目录入口做大量文件发布前，或下一次修改上述目录提交/预览构造函数时。

## 目标

远端 `writer commit --dir ... --bulk` 提交的 ChangeSet 明确携带 bulkIngest=true，未指定时保持 false。

## 依据

- [对象写入](../../doc/design/object.md)：经过 Writer 的单仓提交、并发和幂等语义。
- [Snapshot](../../doc/design/snapshot.md)：介质能力和条件提交边界。
- [Writer 合同](../../object/writer/README.md)：显式 bulk 传输选项与不支持时拒绝。

## 验收

- 远端 CLI 目录提交的 HTTP 请求在 bulk 开/关两种情形分别携带正确值。
- 未变化过滤不丢失传输选项；--changeset 和本地目录既有行为保持。
- 缺少 bulk 能力的介质通过远端目录入口得到明确能力不足，不静默采用普通写入。
- 受影响回归、make check-docs、make quality 通过；真实部署与性能验证单独记录。

## 前置

无。

## 不做

不新增压缩包上传入口，不绕过 Writer，不自动拆批，不承诺未经实测的大仓发布耗时。

## 关闭

并入 `WRITER-INITIAL-UPLOAD-01`，由同一目录构造链路的修复、远端回归和门禁验收闭环。
