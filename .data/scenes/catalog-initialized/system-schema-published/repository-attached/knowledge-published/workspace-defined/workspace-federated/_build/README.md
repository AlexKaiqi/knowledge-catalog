# workspace-federated

同一知识集里两个成员仓可以持有相同 `object_id`。`read --workspace` 返回两条 FederatedValue，不按 scope 覆盖。

构建与探：`TestT11FederatedReadDoesNotOverride`。
