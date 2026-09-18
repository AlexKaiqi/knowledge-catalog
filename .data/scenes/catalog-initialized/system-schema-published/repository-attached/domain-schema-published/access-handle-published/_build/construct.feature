# access-handle-published：Bound State 声明在 Domain Schema origin 上。
# 实体对象已经有身份；不另存 null Aspect。未起 runtime 时 access 是 TEMPORARY_UNAVAILABLE。
# ResourceDescriptor 仍是 invoke 的知识对象。

Feature: access-handle-published

  Scenario: construct
    When I run `kc writer put --command-id publish-orders-health-schema --repo kr://scene/knowledge --object schema/service.health --value '{"entity":"Service","aspect":"health","origin":"http://127.0.0.1:9","fields":{"status":{"type":"string"}}}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc writer put --command-id publish-orders-service --repo kr://scene/knowledge --object Service:orders --aspect properties --value '{"name":"orders"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc binding show --repo kr://scene/knowledge --object Service:orders --aspect health`
    Then the output has:
      | mode   | state |
      | origin | http://127.0.0.1:9 |
    When I run `kc writer put --command-id publish-orders-sql --repo kr://scene/knowledge --object resource/orders-sql --value '{"kind":"ResourceDescriptor","runtime":"sql","protocol":"resource-access/v1","access":{"query":{"call":"sql.query"}}}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc read --repo kr://scene/knowledge --object resource/orders-sql`
    Then the output has:
      | objectId | resource/orders-sql |
      | value.kind          | ResourceDescriptor |
      | value.runtime       | sql |
