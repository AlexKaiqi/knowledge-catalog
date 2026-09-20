# dataset-mounted：迁移后的参考说明

此文件保留原说明，不是状态、可执行 scene 或验证证据。实际 Oracle 由宿主 `_meta.yaml` 中的具名 Go 测试或独立 probe 承接。

# dataset-mounted

`kcfs` 把 File Gateway 计划投影成本机只读目录。产品 argv 是 `--dataset` + `--root`；一次进程只 Resolve 一次已发布 Dataset。上游 HEAD 推进不改 bytes；同一修订重启仍是发布冻 commit，要新数据须 `dataset define --revision` 再挂。周围用户工作目录仍可写，用普通 `ls/rg/cat`。

Go 测试钉命令面与「禁止扫描知识仓伪造 checkout」。Linux `/dev/fuse` 真实生命周期是 `make test-kcfs-e2e`，缺 FUSE 不能在 Agent runner 里伪装 PASS。

构建与探：`TestKnowledgeSetFSPublicCommandAndUsageSurface`、`TestKnowledgeOnlyWorkspaceCannotCheckoutByScanning`。
