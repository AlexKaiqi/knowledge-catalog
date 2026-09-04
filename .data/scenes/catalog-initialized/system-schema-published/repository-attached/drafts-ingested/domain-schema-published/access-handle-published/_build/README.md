# access-handle-published

Snapshot 仓里发布的是**稳定访问句柄**（inline State Binding 或 ResourceDescriptor），不是表权限这类易变当前值。

当前值经统一认证、按 pin 上的声明走 `resource-access/v1` / `StateLookup` 拉取。凭证、endpoint、runtime 不进知识正文，也不在 Catalog 里「注册 connector」。

构建与探：`TestResolveInlineStateAndStreamBindings`、`TestResolveDescriptorBindingAtPinnedCommit`、`TestCatalogViewsChecksAndKnowledgeResolve`。
