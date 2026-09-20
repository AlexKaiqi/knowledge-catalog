# resource-access-granted：迁移后的参考说明

此文件保留原说明，不是状态、可执行 scene 或验证证据。实际 Oracle 由宿主 `_meta.yaml` 中的具名 Go 测试或独立 probe 承接。

# resource-access-granted

持有 `resource.access`，按固定 pin 上的声明调用墙外 Resource Access。容器与 `resource-access/v1` 实现在父节点 `_materials/accessor/`。KC 的仓读权不能替代源系统当场强制；Resource Access 拒绝不得回退到仓内 null 占位。

构建与探：`TestResolveDescriptorBindingAtPinnedCommit`、`TestCatalogViewsChecksAndKnowledgeResolve`。
