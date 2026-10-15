# semantic-object-published：仓发表指标实体，关系仓发表指向该实体的关系。

Feature: semantic-object-published

  Scenario: construct
    When I run `kc writer put urn:kc:object:metric/gmv#/definition --repo kr://scene/object --command-id publish-metric-gmv --schema-ref schema/metric.definition --file $materials/metric.gmv.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:metric/gmv#/definition`
    Then the output has:
      | objectId | metric/gmv |
      | repository          | kr://scene/object |
      | value.name          | Gross merchandise value |
      | value.expression    | SUM(l_extendedprice * (1 - l_discount)) |
      | value.unit          | CNY |
      | value.measureKey    | unique-measure-token-zz9 |
    When I run `kc writer put urn:kc:object:rel/defines/gmv --repo kr://scene/graph --command-id publish-rel-defines-gmv --kind Relation --schema-ref schema/core/relation/v1 --file $materials/rel.defines.gmv.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/graph |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:rel/defines/gmv`
    Then the output has:
      | objectId | rel/defines/gmv |
      | repository          | kr://scene/graph |
      | value.relationType  | defines |
      | schemaRef           | schema/core/relation/v1 |
      | value.endpoints.0.object | metric/gmv |
      | value.endpoints.1.object | schema/metric.definition |
