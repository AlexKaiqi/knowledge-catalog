# 治理任务服务

提案、固定 Preview、结构验证、验证见证与条件合并由各 typed Executor 调用已有 Controlplane/Writer/Gate 端口。请求规则只有一份；传输只解析和组装，不复制提交代数或跨仓事务。

导出形状由 `surface_anchor_test.go` 保持闭集。
