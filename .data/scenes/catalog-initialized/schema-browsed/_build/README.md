# schema-browsed

`kc schema list` 在固定 commit 上分页点名该仓已发布实体。这不是对象 LIST，也不是 SEARCH 空查询，也不是 `schema describe` 或 READ 合同。

系统仓 `kr://kc/system` 不依赖 Workspace。领域仓浏览见 `TestCatalogViewsChecksAndKnowledgeResolve`。

构建与探：`TestSystemSchemaDiscoveryIsBoundedAndWorkspaceIndependent`。
