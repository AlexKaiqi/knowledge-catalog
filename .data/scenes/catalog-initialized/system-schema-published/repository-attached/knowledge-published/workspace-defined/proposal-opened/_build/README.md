# proposal-opened

`governance proposal create` 之后 candidate 存在，main 不动。`preview` / `validate` / `validation record` 与 `operations gate-*` 都在这条世界上。Hook 成功不等于 gate 满足。

构建与探：`TestT9ProposalDoesNotMoveMain`、`TestHookAndGateConfigCRUD`、`TestPreMergeDoesNotSatisfyGate`、`cli/user_journey_test.go`。
