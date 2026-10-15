Feature: Descriptor 已发表但未声明所调用的操作时返回 CAPABILITY_UNSATISFIED

  Scenario: Descriptor 已发表但未声明所调用的操作时返回 CAPABILITY_UNSATISFIED
    When I run `kc writer put urn:kc:object:resource/orders-sql --repo kr://scene/object --command-id publish-orders-sql --value '{"kind":"ResourceDescriptor","origin":"http://127.0.0.1:9","mode":"state","runtime":"sql","protocol":"resource-access/v1","access":{"query":{"call":"sql.query"}}}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:resource/orders-sql`
    Then the output has:
      | objectId | resource/orders-sql |
      | value.kind          | ResourceDescriptor |
      | value.runtime       | sql |
    When I run `kc invoke --repo kr://scene/object --object resource/orders-sql --operation lookup --input '{}'`
    Then error CAPABILITY_UNSATISFIED
