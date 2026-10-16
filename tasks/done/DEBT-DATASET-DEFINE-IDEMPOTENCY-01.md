# DEBT-DATASET-DEFINE-IDEMPOTENCY-01 · Dataset 定义重试不幂等

## 债

- 位置：`invocation/verbs_dataset.go`（原 `verbs_catalog.go`）的 Dataset 定义；内容派生的 command-id `dataset-define:<setID>:<revision>:<hash>` 没接命令账本，版本校验在账本重放之前短路。
- 信号：响应丢失后重试同一已接受的发布，返回 `NON_FAST_FORWARD` 而不是幂等确认（`cli/deployment_dataset_recovery_test.go` 实测）；和 Writer PUT 的 `REPLAYED` 不一致。
- 判定：真缺口，低危：版本单调守卫本身正确。
- 处置：先查账本命中重放再做版本校验，或“修订号等于当前且配方相同”返回幂等结果；按 Writer 账本的所有权取其一。
- 触发条件：下次触碰 Dataset 发布路径，即 `DATASET-REVISION-01`（自动跟随会大量重试发布）。原条目 `CAP-ORG-01` 的 DATASET-DEFINE-IDEMPOTENCY。

## 目标

重试同一个已接受的 Dataset 发布得到幂等确认，不产生新修订。

## 依据

- [Dataset](../../doc/design/dataset.md)、[服务](../../doc/design/service.md)。

## 验收

- 恢复旅程测试里重试返回幂等确认；配方不同的同修订号仍被拒绝。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

无；建议和 `DATASET-REVISION-01` 一起做。

## 不做

不放宽版本单调检查。

## 关闭

2026-10-16 并入 `DATASET-REVISION-01`：位置、信号与剩余验收在该任务的“并入”一节继续推进。本文件已有的交付与证据保留为历史，不代表验收通过。
