Feature: Binding 已发表但 runtime 不可达时返回 TEMPORARY_UNAVAILABLE

  Scenario: Binding 已发表但 runtime 不可达时返回 TEMPORARY_UNAVAILABLE
    When I run `kc writer put urn:kc:object:schema/service.health --repo kr://scene/object --command-id publish-orders-health-schema --value '{"entity":"Service","aspect":"health","fields":{"status":{"type":"string"}}}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc writer put urn:kc:object:service/orders#/properties --repo kr://scene/object --command-id publish-orders-service --value '{"name":"orders"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc writer put urn:kc:object:resource/orders-health --repo kr://scene/object --command-id publish-orders-health-descriptor --value '{"kind":"ResourceDescriptor","origin":"http://127.0.0.1:9","mode":"state","runtime":"http","protocol":"resource-access/v1","access":{"lookup":{"call":"lookup"}}}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc writer put urn:kc:object:service/orders#/health --repo kr://scene/object --command-id publish-orders-health-binding --schema-ref schema/service.health --value-source '{"kind":"binding","binding":{"descriptorRef":"resource/orders-health"}}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc binding show --repo kr://scene/object --object service/orders --aspect health`
    Then the output has:
      | mode          | state |
      | origin        | http://127.0.0.1:9 |
      | descriptorRef | resource/orders-health |
    When I run `kc access --repo kr://scene/object --object service/orders --aspect health`
    Then error TEMPORARY_UNAVAILABLE
