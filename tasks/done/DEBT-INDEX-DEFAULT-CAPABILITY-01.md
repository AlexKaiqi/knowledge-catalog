# DEBT-INDEX-DEFAULT-CAPABILITY-01 · 默认索引成员与实例实际能力不一致

## 债

- 位置：`declarativeindex/capability/manifest.go`、默认 `metric-text` / `metric-unit` 声明及解释器的模板、输入缺省策略与关键词规范化构建。
- 信号：恢复真实部署后的 `accept-search-capability-catalog` 进入产品断言，默认成员返回 unsupported；独立原文件 Trial 证实 `metric-text` 因模板不可用、`metric-unit` 因关键词规范化不可用，另两默认成员 supported。原始结果见 `.validation/runs/20261016-environment-recovery/capability-diagnostics/index.json`，批跑见该目录 `acceptances/20261016T024552Z-8739a7f83860/acceptance.json`。
- 判定：原规格要求默认四成员逐个通过，当前实例如实拒绝尚未实现的选项，属于实现缺口；不是 OOM、部署缺失或可通过修改期待值消除的失败。
- 处置：与 `INDEX-DECLARATION-01` 的既有五条部署验收同一目标、同一索引实现和环境，登记后立即并入。实现须与主索引改动衔接，不能只把 manifest 的拒绝标记删掉来宣称支持。
- 触发条件：本任务真实部署收口，或 `INDEX-SEARCH-SPACE-01` 进入 done 前。

## 目标

默认四成员使用的公开构建选项有真实解释和构建行为；原文件试运行通过，声明外的能力仍明确拒绝。

## 依据

- [发现](../../doc/design/discovery.md)。
- [插件](../../doc/design/plugins.md)。

## 验收

- 保留原规格和默认四份声明，`accept-search-capability-catalog` 全部 then 通过；模板、缺省输入和规范化均验证实际构建行为。
- HEAD / 索引无试运行副作用、逐成员隔离与清单外拒绝回归不退化；受影响合同、文档和质量门禁全绿。

## 前置

与主索引实现划清共享文件后执行。

## 不做

不删默认选项，不把预期 supported 改成 unsupported，不只修改清单承诺。

## 关闭

2026-10-16 登记后立即并入 `INDEX-DECLARATION-01`；原文件与真实返回证据保留，剩余实现与全部部署验收由主任务收口。
