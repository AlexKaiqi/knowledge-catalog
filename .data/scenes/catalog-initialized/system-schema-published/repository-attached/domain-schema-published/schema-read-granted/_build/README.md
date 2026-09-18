# schema-read-granted

持有仓级 `knowledge.schema.read`：`schema list` 看有哪些实体，`schema describe` 看 AccessHints。不被 `catalog.read` 隐含，也不放行实例正文。

构建与探：`TestDescribeSchemaListsAccessHints`、`TestSystemSchemaDiscoveryIsBoundedAndWorkspaceIndependent`。
