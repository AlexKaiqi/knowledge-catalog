# DEBT-INDEX-MANIFEST-NUMERIC-01 · 自定义清单数值选项与检查分支不一致

## 债

- 位置：`declarativeindex/capability/manifest.go` 的 `checkOption`，以及自定义 `Manifest` 的 `IndexBuildOption.Type`。
- 信号：2026-10-16 只读反例将实例的构建选项声明为 `number` 或 `integer`；公开 JSON 往返后，数值 `2` 被拒为“must be a non-empty string”，而字符串 `"2"` 被接受。默认清单没有开放这类构建选项，未证明默认清单的既有路径受影响。
- 判定：实例公开的类型与其检查行为不一致；不能把仅检查 `Rebuilds` 的现有数值选项测试当成 `Check` 已覆盖。
- 处置：并入 `INDEX-DECLARATION-01`，在能力清单所属文件范围明确后补类型回归并修复或明确拒绝无法实现的类型；不在本轮 L2 三个入口之外改代码。
- 触发条件：`INDEX-DECLARATION-01` 能力清单收尾；该任务当前已在 `doing/`，立即归并，不另开并行代码线。

## 目标

实例能力清单公开的选项类型与逐成员检查一致；合法数值和错误字符串有相反的明确判断。

## 依据

- [发现](../../doc/design/discovery.md)：能力清单按实例公开，并按清单逐成员核对。
- [插件](../../doc/design/plugins.md)：实例清单之外的写法明确不支持。

## 验收

- 数值选项的公开 JSON 往返、数值与字符串反例分别先红再绿；保留默认清单的物理能力闭集与重建规则断言。
- `make check-docs`、`make quality` 和受影响合同通过。

## 前置

并入 `INDEX-DECLARATION-01`。

## 不做

不新增默认清单能力，不为让自定义试运行通过而开放物理编译器尚未实现的控制。

## 关闭

2026-10-16 并入 `INDEX-DECLARATION-01`。只读反例与源码快照：`.validation/runs/20261016T015959Z-l2-index-trial/manifest_numeric_review_test.go`；执行报告登记在同目录门禁索引。此关闭仅表示归并，未修复、未验收。
