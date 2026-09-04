# workspace-resolve-granted

持有 `workspace.resolve`：解析本次 pin。不发权、不读正文。向调用方交出完整成员读侧元数据时，仍要求全部成员的 `knowledge.read`。Pin 固定数据坐标，不冻结授权。

构建与探：`TestOpenedWorkspacePinDoesNotMoveWithLaterCommit`、`TestWorkspaceAuthorizationCoverageIsHonest`。
