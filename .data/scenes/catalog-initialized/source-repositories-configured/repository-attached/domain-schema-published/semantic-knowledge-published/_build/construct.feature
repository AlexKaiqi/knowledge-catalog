# semantic-knowledge-published：知识仓发表指标实体，关系仓发表指向该实体的关系。

Feature: semantic-knowledge-published

  Scenario: construct
    When I run `kc writer put --command-id publish-metric-gmv --repo kr://scene/knowledge --object metric/gmv --aspect definition --schema-ref schema/metric.definition --file $materials/metric.gmv.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc read --repo kr://scene/knowledge --object metric/gmv --aspect definition`
    Then the output has:
      | objectId | metric/gmv |
      | repository          | kr://scene/knowledge |
      | value.name          | Gross merchandise value |
      | value.expression    | SUM(l_extendedprice * (1 - l_discount)) |
      | value.unit          | CNY |
      | value.measureKey    | unique-measure-token-zz9 |
    When I run `kc writer put --command-id publish-rel-defines-gmv --repo kr://scene/graph --object rel/defines/gmv --schema-ref schema/core/relation/v1 --file $materials/rel.defines.gmv.json`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/graph |
      | result.newCommit    | nonempty |
    When I run `kc read --repo kr://scene/graph --object rel/defines/gmv`
    Then the output has:
      | objectId | rel/defines/gmv |
      | repository          | kr://scene/graph |
      | value.relationType  | defines |
