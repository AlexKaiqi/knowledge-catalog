# file-view-planned

Workspace File Gateway 已按固定 pin 给出 path/tree/blob 计划（含 semantic YAML 视图）。`kc local workspace overlay` 改配方，不写知识。此时尚未 FUSE 挂载。

知识仓无显式 mount 时不得扫描伪造工作树。

构建与探：`TestWorkspaceFileGatewayBuildsSemanticYAMLViewWithoutRepositoryMountPaths`、`TestWorkspaceFileGatewayPagesDirectChildrenAndReadsFixedRange`、`TestPrepareRemoteWorkspaceFSUsesGatewayAndKeepsFixedPin`、`TestLoomOverlayAndBaseRev`。
