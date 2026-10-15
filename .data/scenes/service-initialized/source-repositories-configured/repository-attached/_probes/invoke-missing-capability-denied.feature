Feature: Descriptor 已发表但调用能力缺失时返回 CAPABILITY_UNSATISFIED

  Scenario: Descriptor 已发表但调用能力缺失时返回 CAPABILITY_UNSATISFIED
    When I run `kc writer put urn:kc:object:resource/orders-sql --repo kr://scene/object --command-id publish-orders-sql --value '{"kind":"ResourceDescriptor","runtime":"sql","protocol":"resource-access/v1","access":{"query":{"call":"sql.query"}}}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:resource/orders-sql`
    Then the output has:
      | objectId | resource/orders-sql |
      | value.kind          | ResourceDescriptor |
      | value.runtime       | sql |
    When I run `kc invoke --repo kr://scene/object --object resource/orders-sql --operation query --input '{}'`
    Then error CAPABILITY_UNSATISFIED
