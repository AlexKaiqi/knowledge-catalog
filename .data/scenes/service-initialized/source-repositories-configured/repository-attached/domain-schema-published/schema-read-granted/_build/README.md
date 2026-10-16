# schema-read-granted

持有仓级 `object.schema.read`：`schema list` 看有哪些实体，`schema describe` 看字段结构与约束；检索能力由独立索引声明提供。不被 `inventory.read` 隐含，也不放行实例正文。

构建与探：`TestDescribeSchemaListsStructureWithoutAccess`、`TestSystemSchemaDiscoveryIsBoundedAndWorkspaceIndependent`。
