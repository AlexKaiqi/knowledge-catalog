# domain-schema-published：接入方把草稿目录作为期望正文提交。

Feature: domain-schema-published

  Scenario: construct
    When I run `kc writer commit --command-id publish-domain-schema --repo kr://scene/knowledge --dir $materials/drafts`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc schema list --repo kr://scene/knowledge`
    Then the output has:
      | repository   | kr://scene/knowledge |
      | continuation | absent |
    Then the output includes:
      | schemas[].objectId | schema/metric.definition |
      | schemas[].entity      | Metric |
      | schemas[].description | 业务指标的定义。 |
    When I run `kc writer commit --command-id publish-relation-schema --repo kr://scene/graph --dir $materials/graph-drafts`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/graph |
      | result.newCommit    | nonempty |
    When I run `kc schema list --repo kr://scene/graph`
    Then the output includes:
      | schemas[].objectId | schema/core/relation/v1 |
      | schemas[].entity | Relation |
