# proposal-opened

`governance proposal create` 之后 candidate 存在，main 不动。`operations gate-*` 停在本节点上探。Preview / validate / record / merge 是后续分叉，各只做进入该状态的那一步。

构建与探：`TestT9ProposalDoesNotMoveMain`、`TestHookAndGateConfigCRUD`、`TestPreMergeDoesNotSatisfyGate`、`cli/user_journey_test.go`。
