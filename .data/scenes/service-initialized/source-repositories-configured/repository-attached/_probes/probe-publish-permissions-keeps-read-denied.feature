# permissions Aspect 是来源授权对象；本用例发表的 Aspect 不成为其它用例的前态。

Feature: 发表并回读来源权限 Aspect 后对应主体读取仍被拒绝

  Scenario: 发表并回读来源权限 Aspect 后对应主体读取仍被拒绝
    When I run `kc writer put urn:kc:object:table/orders#/permissions/user:bob --repo kr://scene/object --command-id publish-table-permissions --file $materials/table.orders.permissions.json --kind Member`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:table/orders#/permissions/user:bob`
    Then the output has:
      | objectId | table/orders |
      | value.privileges.0  | SELECT |

    When I run `kc read urn:kc:object:table/orders --as bob`
    Then error FORBIDDEN
