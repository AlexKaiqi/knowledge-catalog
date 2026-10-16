# DEBT-DISCOVERY-SEARCH-SIZE-01 · Dataset 搜索文件体积门禁

## 债

- 位置：`discovery/service/dataset_search.go`。
- 信号：2026-10-16 宣传片交付时执行 `make quality`，输出 `709 exceeds 700 lines`。
- 判定：仓库全局文件体积门禁的真实失败；本次动画工作未修改该文件，不据此推断搜索行为错误。
- 处置：保留门禁，由维护该文件的产品任务按职责整理文件边界；不以压缩行距、提高阈值或跳过检查解决。
- 触发条件：下一次修改该文件或处理此门禁失败时并入相应任务。`CONSUME-BASELINE-01` 和 `INDEX-SEARCH-SPACE-01` 涉及相邻领域，但尚无证据确认本信号已被其中某个任务承接；本记录不启动并行产品改动。

## 目标

Dataset 搜索文件职责清楚并满足现有体积门禁，搜索和读取合同保持。

## 依据

[工程质量闭环](../../doc/engineering/quality.md)。具体设计范围由实际修复任务依据改动确定。

## 验收

现有文件体积门禁通过，受影响的搜索与读取测试通过，`make check-docs`、`make quality` 通过；不削弱断言，不 skip。

## 前置

确认负责修改该文件的任务，避免与在途产品工作重叠。

## 不做

不在宣传片任务中调整产品行为、公开接口、质量阈值或测试范围。

## 交付

- 实现：`CONSUME-BASELINE-01` 修改该文件时承接本信号。`datasetBasis`（Dataset 修订依据）按职责移到 `discovery/service/dataset_basis.go`，`dataset_search.go` 回到 700 行；门限与断言未动。
- 已验证（2026-10-16）：`make quality` 通过（文件体积门禁含在内，证据 `.validation/runs/20261016T0230Z-consume-baseline/kc-quality.log`）；`go test ./discovery/...` 全绿；`make check-docs` 通过。
- 未提交。
