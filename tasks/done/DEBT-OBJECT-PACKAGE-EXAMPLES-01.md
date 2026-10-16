# DEBT-OBJECT-PACKAGE-EXAMPLES-01 · 更新对象包说明中的过期发布示例

## 债

- 位置：`object/format/README.md` 的 `kc ingest --dir`；`object/writer/README.md` 的 Schema 发布和索引声明示例。
- 信号：2026-10-15 公开帮助走查发现，前者指向已不在公开命令表中的 ingest；后者仍用 `writer put --object`、Schema 顶层 origin 与字段 access。
- 判定：示例与当前命令参数及结构 Schema 合同不符；不能用于接入方直接执行。
- 处置：按所属公开命令改写包说明，索引与取值采用各自声明，补可执行示例验证。
- 触发条件：继续对象包文档维护或 Schema/索引迁移收尾时。

## 目标

对象包 README 中的操作示例与公开输入合同一致，复制示例不会进入已删除的入口。

## 依据

- [对象](../../doc/design/object.md)、[Affordance](../../doc/design/affordance.md)。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 说明引用的发布命令可实际执行，Schema 不混入索引与取值字段。
- 对应测试、`make check-docs` 与 `make quality` 通过。

## 前置

无。

## 不做

不恢复旧命令，不改变对象协议或缩减索引、取值能力。

## 关闭

2026-10-16 并入 `PUBLICATION-SYNC-01`：位置、信号与剩余验收在该任务的“并入”一节继续推进。本文件已有的交付与证据保留为历史，不代表验收通过。
