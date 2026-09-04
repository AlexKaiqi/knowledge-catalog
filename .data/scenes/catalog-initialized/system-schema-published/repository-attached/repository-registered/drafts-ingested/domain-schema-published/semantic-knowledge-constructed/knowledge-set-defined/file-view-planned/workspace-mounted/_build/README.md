# workspace-mounted

`kcfs` 把 File Gateway 计划投影成本机只读目录。一次进程只 Resolve 一次 Workspace；上游 HEAD 推进不改 bytes。周围用户工作目录仍可写，用普通 `ls/rg/cat`。

Go 测试钉命令面与「禁止扫描知识仓伪造 checkout」。Linux `/dev/fuse` 真实生命周期是 `make test-kcfs-e2e`，缺 FUSE 不能在 Agent runner 里伪装 PASS。

构建与探：`TestWorkspaceFSPublicCommandAndUsageSurface`、`TestKnowledgeOnlyWorkspaceCannotCheckoutByScanning`。
