# permissions Aspect 是来源授权知识；本用例发表的 Aspect 不成为其它用例的前态。

Feature: 发表并回读来源权限 Aspect 后对应主体读取仍被拒绝

  Scenario: 发表并回读来源权限 Aspect 后对应主体读取仍被拒绝
    When I run `kc writer put --command-id publish-table-permissions --repo kr://scene/knowledge --object Table:orders --aspect permissions --member user:bob --file $materials/table.orders.permissions.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc read --repo kr://scene/knowledge --object Table:orders --aspect permissions --member user:bob`
    Then the output has:
      | objectId | Table:orders |
      | value.privileges.0  | SELECT |

    When I run `kc read --as bob --repo kr://scene/knowledge --object Table:orders`
    Then error FORBIDDEN
