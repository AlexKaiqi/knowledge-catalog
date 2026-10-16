# DEBT-INDEX-UNUSED-IMPORT-01 · 检索命令改动中的未使用导入阻塞质量检查

## 债

- 位置：`invocation/verbs_index.go:13`。
- 信号：2026-10-16 动画调整后的 `make quality` 报 `"kc/discovery" imported as retrieval and not used`。
- 判定：当前工作区的产品改动尚在进行；该编译问题不来自动画变更，但阻塞仓库质量检查。
- 处置：并入改同一批文件的 `INDEX-SEARCH-SPACE-01`，随检索命令改动收口，不由动画任务修改产品文件。
- 触发条件：`INDEX-SEARCH-SPACE-01` 已在 `doing/`，立即归并。

## 目标

检索命令相关文件可编译，仓库质量检查通过。

## 依据

[工程质量闭环](../../doc/engineering/quality.md)。

## 验收

`make quality` 不再因该未使用导入失败，并完成主任务约定的测试。

## 前置

`INDEX-SEARCH-SPACE-01`。

## 不做

不修改动画时间线，不在动画任务里接管产品实现。

## 关闭

归并到进行中的 `INDEX-SEARCH-SPACE-01`，剩余修复和验收由主任务承接；关闭表示归并，不表示问题已修复。
