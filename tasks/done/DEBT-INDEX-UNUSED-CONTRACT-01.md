# DEBT-INDEX-UNUSED-CONTRACT-01 · 清理检索范围迁移后未使用的合同适配函数

## 债

- 位置：`discovery/service/search.go:25` 的 `projectedAccessContracts`。
- 信号：2026-10-16 Realcase 候选校验复用 `make quality`，staticcheck 报 `U1000`，该函数已无调用者。
- 判定：检索范围迁移进行中的工作区质量缺口，不属于 Realcase 数据转换代码。
- 处置：并入已在进行的 `INDEX-SEARCH-SPACE-01`，由该任务按最终调用关系清理或恢复必要调用，不弱化检查。
- 触发条件：`INDEX-SEARCH-SPACE-01` 收口当前 `discovery/service` 调用关系及统一质量门禁时。

## 目标

检索范围迁移后的服务代码没有遗留未使用适配函数，统一质量门禁恢复通过。

## 依据

[工程质量闭环](../../doc/engineering/quality.md)、[发现](../../doc/design/discovery.md)。

## 验收

staticcheck 不再报告该函数未使用；`make check-docs`、`make quality` 与受影响的发现服务合同通过。

## 前置

随 `INDEX-SEARCH-SPACE-01` 当前工作区收口，不另起并行修改。

## 不做

不修改 Realcase 原始材料、候选 Schema 或转换产物；不删除或削弱质量守卫。

## 关闭

登记时 `INDEX-SEARCH-SPACE-01` 已在 doing，按任务归并规则立即并入该任务。
检查证据为 `.data/realcase/proposal/quality-final.log`；本条关闭仅表示归并，不表示问题已修复或质量门禁通过。
