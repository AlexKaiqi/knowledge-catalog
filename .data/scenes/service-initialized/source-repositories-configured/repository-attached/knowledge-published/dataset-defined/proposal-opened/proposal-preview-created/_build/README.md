# proposal-preview-created

本节点保存绑定知识集固定基线和候选提交的 Preview。结构检查用例取得 PASSED 报告并确认 main 未变；另一用例独立记录外部 PASSED 报告，再用该报告合并并读回 proposed。两条用例不共享临时报告。

`TestT9MergeDoesNotNeedPromote` 与 `TestT9MergeRejectsMovedMain` 保留为独立 Go 证据，分别验证合并无需 promote 与主分支移动时拒绝合并。
