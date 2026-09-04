# domain-schema-published：接入方 COMMIT 已 ingest 的 Domain Schema。

Feature: domain-schema-published

  Scenario: construct
    When I run `kc writer commit --command-id publish-domain-schema --changeset $home/schema.changeset.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc writer receipt --command-id publish-domain-schema`
    Then the output has:
      | commandId | publish-domain-schema |
      | digest    | nonempty |
    When I run `kc knowledge schema browse --repo kr://scene/knowledge`
    Then the output has:
      | repository        | kr://scene/knowledge |
      | exhausted         | true |
      | coverage.complete | true |
    Then the output includes:
      | schemas[].objectId | schema/metric.definition |
      | schemas[].entity   | Metric |
