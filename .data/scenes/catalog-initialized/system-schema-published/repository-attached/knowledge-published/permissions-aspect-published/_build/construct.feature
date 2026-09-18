# permissions-aspect-published：外部 GRANT 快照已进仓。公开 `kc writer put`。
# 首次 construct 的 disposition 是 APPLIED；同一 --command-id 再走是 REPLAYED，对象仍在 HEAD。

Feature: permissions-aspect-published

  Scenario: construct
    When I run `kc writer put --command-id publish-table-permissions --repo kr://scene/knowledge --object Table:orders --aspect permissions --member user:bob --file $materials/table.orders.permissions.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc read --repo kr://scene/knowledge --object Table:orders --aspect permissions --member user:bob`
    Then the output has:
      | objectId | Table:orders |
      | value.privileges.0  | SELECT |
