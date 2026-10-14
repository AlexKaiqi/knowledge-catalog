Feature: Binding 已发表但 runtime 不可达时返回 TEMPORARY_UNAVAILABLE

  Scenario: Binding 已发表但 runtime 不可达时返回 TEMPORARY_UNAVAILABLE
    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:schema%2Fservice.health --command-id publish-orders-health-schema --value '{"entity":"Service","aspect":"health","origin":"http://127.0.0.1:9","fields":{"status":{"type":"string"}}}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:Service%3Aorders#/properties --command-id publish-orders-service --value '{"name":"orders"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc binding show --repo kr://scene/knowledge --object Service:orders --aspect health`
    Then the output has:
      | mode   | state |
      | origin | http://127.0.0.1:9 |
    When I run `kc access --repo kr://scene/knowledge --object Service:orders --aspect health`
    Then error TEMPORARY_UNAVAILABLE
