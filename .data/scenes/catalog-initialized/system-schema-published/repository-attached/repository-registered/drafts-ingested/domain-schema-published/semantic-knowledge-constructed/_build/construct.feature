# semantic-knowledge-constructed：按已发布 Schema 约束 PUT 实例。

Feature: semantic-knowledge-constructed

  Scenario: construct
    When I run `kc writer put --command-id publish-metric-gmv --repo kr://scene/knowledge --object metric/gmv --aspect definition --schema-ref schema/metric.definition --file $materials/metric.gmv.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc knowledge read --repo kr://scene/knowledge --object metric/gmv --aspect definition`
    Then the output has:
      | knowledgeRef.object | metric/gmv |
      | repository          | kr://scene/knowledge |
      | value.name          | Gross merchandise value |
      | value.expression    | SUM(l_extendedprice * (1 - l_discount)) |
      | value.unit          | CNY |
      | value.measureKey    | unique-measure-token-zz9 |
