# semantic-knowledge-published：知识仓发表指标实体，关系仓发表指向该实体的关系。

Feature: semantic-knowledge-published

  Scenario: construct
    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:metric%2Fgmv#/definition --command-id publish-metric-gmv --schema-ref schema/metric.definition --file $materials/metric.gmv.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:metric%2Fgmv#/definition`
    Then the output has:
      | objectId | metric/gmv |
      | repository          | kr://scene/knowledge |
      | value.name          | Gross merchandise value |
      | value.expression    | SUM(l_extendedprice * (1 - l_discount)) |
      | value.unit          | CNY |
      | value.measureKey    | unique-measure-token-zz9 |
    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fgraph:rel%2Fdefines%2Fgmv --command-id publish-rel-defines-gmv --kind Relation --schema-ref schema/core/relation/v1 --file $materials/rel.defines.gmv.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/graph |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fgraph:rel%2Fdefines%2Fgmv`
    Then the output has:
      | objectId | rel/defines/gmv |
      | repository          | kr://scene/graph |
      | value.relationType  | defines |
      | schemaRef           | schema/core/relation/v1 |
      | value.endpoints.0.objectRef.repository | kr://scene/knowledge |
      | value.endpoints.1.objectRef.object | schema/metric.definition |
