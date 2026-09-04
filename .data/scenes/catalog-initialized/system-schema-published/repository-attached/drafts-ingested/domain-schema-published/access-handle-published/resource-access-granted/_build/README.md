# resource-access-granted

持有 `resource.access`，按固定 pin 上的 ResourceDescriptor 调用墙外 runtime。KC 的仓读权不能替代源系统当场强制；runtime 拒绝不得回退到仓内 null 占位。

构建与探：`TestResolveDescriptorBindingAtPinnedCommit`、`TestCatalogViewsChecksAndKnowledgeResolve`。
