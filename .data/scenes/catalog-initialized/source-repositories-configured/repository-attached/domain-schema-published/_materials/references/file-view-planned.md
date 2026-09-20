# file-view-planned：迁移后的参考说明

此文件保留原说明，不是状态、可执行 scene 或验证证据。实际 Oracle 由宿主 `_meta.yaml` 中的具名 Go 测试或独立 probe 承接。

# file-view-planned

File Gateway 已按 Dataset 发布时冻住的清单给出 path/tree/blob 计划（含 semantic YAML 视图）。`kc dataset overlay` 在客户端合成临时配方，不改共享定义、不写知识。此时尚未 FUSE 挂载。

知识仓无显式 mount 时不得扫描伪造工作树。

构建与探：`TestWorkspaceFileGatewayBuildsSemanticYAMLViewWithoutRepositoryMountPaths`、`TestWorkspaceFileGatewayPagesDirectChildrenAndReadsFixedRange`、`TestPrepareRemoteKnowledgeSetFSUsesGatewayAndKeepsFixedPin`、`TestLoomOverlayAndBaseRev`。
