# QUESTION-BASELINE-01 · 审计对照基座

## 要回答的问题

本轮的对照基线是什么：4 个 Dataset 的 revision 与 itemCount、11 个仓的 commit、全部 grant 规则、
`registry audit` 条目，以及六个读接口的耗时采样。

## 已知证据

任务书 §三 的基线已漂移，不可作为对照源：`autumn-launch-kit` 记 rev5 实测 rev6；`qinghe-analysis`
记 rev4 实测 rev5；`registry audit` 记 26 条实测 28 条。

首次 `kc show` 曾出现 `Get ".../inventory": EOF` 后转正常，冷启与稳态需分别记录。

## 判定标准

- 基座含 4 个 Dataset、11 个仓、28 条审计条目、全部 grant 规则，逐项带采集时刻，否则不构成基座。
- 每个 Dataset 同时记 revision 与 itemCount，供 itemCount 与可列条目数比对。
- 六个接口各不少于 5 次采样，给出中位数、最大值、失败次数；冷启与稳态分开记录。
- 采集脚本单命令可跑、输出 JSON、字段稳定可 diff；其余探索直接引用，不另建第二套数字。

## 前提

无。

## 不涉及

不修改库存、不写 grant、不对既有 Dataset 做 retire；基座不是验收结论；
不把采集塞进 `make test`，它是独立入口。

## 结论

待填。
