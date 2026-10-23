# DEBT-INDEX-READY-DIAGNOSE-01 · 就绪接口收缩后没有排障字段

## 债

- 位置：`discovery/projection/describe.go` 的现有描述结构（含成员、basis、覆盖与状态），以及
  `server/service_management_routes.go:208` 的 `projectionDescribe`。
- 信号：排障时只知道"没就绪"，不知道缺哪个成员、basis 指到哪个提交、覆盖到哪一步、状态是
  BUILDING 还是 FAILED。
- 判定：这是窄就绪接口换来的确定代价，不是实现疏漏。诊断是低频但信息量大的需求，塞进同一响应会让
  日常查询每次付全量成员核对成本，也会把内部状态名变成对外词。
- 处置：新增 `indexes:describe` 承接诊断，保持 `indexes:ready` 只有 `ready` 与 `objectCount`。
- 触发条件：出现"索引没就绪但说不清原因"的实际排障时。

## 关闭

2026-10-22 并入 `INDEX-SURFACE-01`，未按本文件单独验收。

并入理由：本条改的文件（`declarativeindex/projection/describe.go`、`server/service_management_routes.go`）
与 `INDEX-READY-01` 完全相同，且它的触发条件指向 `INDEX-READY-01` 上线后的第一次归因失败——也就是
`INDEX-SURFACE-01` 交付之后。按债的归并规则（触发条件指向某任务、或与某任务改同一批文件即并入），
本条在该任务开工时一并处理。

承接内容与变化：

- 原"处置"的两个备选方向（保留完整 describe 只改路径、或新设计按需展开的窄诊断）选定为**前者**：
  `indexes:describe` 复用既有完整描述结构，只改路径与对外词汇。理由是窄诊断需要新协议形状，而本轮
  的目标是给索引一个统一对外入口，不是重设诊断协议；留在"按需展开"等于把本条重新挂成一条未来的债。
- 原"验收"五条（回答四个问题、不触发构建、日常就绪成本不上升、对外字段有用户词映射、门禁通过）
  全部移入 `INDEX-SURFACE-01` 的验收。
- 原"不做"三条（不塞回 ready、不恢复 `projections:describe` 旧路径、不加 `reason` 充当半个诊断）
  保留在 `INDEX-SURFACE-01` 的"不做"里。
