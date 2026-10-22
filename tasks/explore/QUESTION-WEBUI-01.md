# QUESTION-WEBUI-01 · 控制台与 CLI 对同一事实是否一致

## 要回答的问题

`snapshot/console` 的 `/ui/` 与 CLI 对 Dataset 修订、成员、投影就绪状态、检索结果、来源 commit
是否给出相同结论；前端是否因交互便利放宽后端门禁；不可区分状态是否被渲染成一个确定结论。

## 已知证据

前端为 Vite + React + Playwright，`test/e2e` 下有 POM 与 6 个用例，`make quality-webui` 跑
typecheck 与 lint。路由入口只剩 `/ui/`（`CONSOLE-RETIRE-01` 已退役 `/console` 与跳转页）。
前端自述见 `snapshot/console/webui/CONSOLE-VALIDATION.md`、`DATASET-API-REVIEW.md`、
`ACCESS-CONTROL-API-REVIEW.md`。

## 判定标准

- 三类事实在 `/ui/` 与 `kc` 输出逐项比对，差异全部可归因，关闭为"一致"；否则关闭为"确认缺陷"。
- 无权主体在 `/ui/` 的可见范围与门禁矩阵一致，关闭为"门禁未被放宽"；出现后端拒绝的数据，
  关闭为"确认缺陷"。
- 检索空结果与索引未就绪在前端有区分或标注未知，关闭为"状态可分辨"；合并成一个空态，
  关闭为"确认缺陷"。

## 前提

`QUESTION-AUTH-01` 的门禁矩阵先出结论，前端以该矩阵为对照，避免两处各判一遍。

## 不涉及

不重写页面结构、不调整视觉、不新增前端路由、不恢复 `/console`、不在前端实现后端缺失的权限判断。

## 结论

待填。
