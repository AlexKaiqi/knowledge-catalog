# proposal-opened

`governance proposal create` 后，候选分支保存 proposed，main 仍是 hi。后继 `proposal-preview-created` 保存可供两个独立用例复用的 Preview：结构检查与外部报告合并分别执行，不传递 probe 结果。

本节点的 Go 证据自行创建夹具，验证提案不推进 main，以及 gate 与 hook 的独立合同。门禁增删用例挂在 `repository-attached`，不依赖提案。
