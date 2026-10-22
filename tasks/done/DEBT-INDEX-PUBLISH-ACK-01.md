# DEBT-INDEX-PUBLISH-ACK-01 · 索引发布确认丢失时不能清除已采用生成代

## 债

- 位置：`declarativeindex/opensearch/internal/opensearchimpl/projection.go` 的 `rebuildSession.Commit`、`Abort`、`dropGeneration`。
- 信号：READY 控制记录的条件写已由 OpenSearch 接受但响应丢失时，Commit 返回错误进入 Abort；
  Abort 不核对权威 ActiveIndex 就删除本次 physicalIndex，可能删除实际已采用的生成代。
- 判定：静态故障时序与持久采用保证冲突，须以响应丢失注入验证；不能把传输错误当成未发布证明。
- 处置：并入正在进行且修改相同文件的 `INDEX-PHYSICAL-01`，先核对权威采用与已准入物理操作，
  未知或仍被引用时保留资源及恢复记录，不报告确认回收。
- 触发条件：该任务修改发布交接、失败恢复或代际回收时立即处理。

## 目标

发布确认丢失后不破坏实际被采用的生成代，恢复核对沿同一物理资源身份执行。

## 依据

[索引物理布局](../../doc/design/index-physical.md)的发布、保护与确认删除保证。

## 验收

真实执行 READY 控制写、丢弃响应，再执行失败恢复；已被权威采用的资源仍可查询，
明确未采用的候选按保护与物理操作结果清理，未知结果不能误报为完成。

## 前置

无。

## 不做

不更换 Dataset 保留策略，不处理与本故障时序无关的存量清理。

## 关闭

并入 `INDEX-PHYSICAL-01`。本条关闭只表示归并，不表示故障已修复或验收通过。
