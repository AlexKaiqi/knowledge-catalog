# access-handle-published：Snapshot 里是稳定 Binding 句柄，不是当前值。

Feature: access-handle-published

  Scenario: construct
    When I run `kc writer put --command-id publish-orders-health --repo kr://scene/knowledge --object Service:orders --aspect health --value null --value-source '{"kind":"binding","binding":{"mode":"state","runtime":"health","protocol":"resource-access/v1","operations":{"lookup":{"call":"health.lookup"}}}}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc knowledge binding show --repo kr://scene/knowledge --object Service:orders --aspect health`
    Then the output has:
      | mode    | state |
      | runtime | health |
    When I run `kc writer put --command-id publish-orders-sql --repo kr://scene/knowledge --object resource/orders-sql --value '{"kind":"ResourceDescriptor","runtime":"sql","protocol":"resource-access/v1","access":{"query":{"call":"sql.query"}}}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc knowledge read --repo kr://scene/knowledge --object resource/orders-sql`
    Then the output has:
      | knowledgeRef.object | resource/orders-sql |
      | value.kind          | ResourceDescriptor |
      | value.runtime       | sql |
