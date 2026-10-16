Feature: 消费者沿同一 Dataset 修订发现可检索字段、检索并回读

  Scenario: 消费者沿同一 Dataset 修订发现可检索字段、检索并回读
    Given local HTTP server
    When I run `kc schema list --dataset scene-set --revision 1 --limit 1 --as agent:copilot --server $server`
    Then the output has:
      | dataset      | scene-set |
      | revision     | 1 |
      | continuation | nonempty |
    When I run `kc schema list --dataset scene-set --revision 1 --continuation $last.continuation --as agent:copilot --server $server`
    Then the output has:
      | dataset      | scene-set |
      | revision     | 1 |
      | continuation | absent |
    When I run `kc schema describe --dataset scene-set --revision 1 --object schema/metric.definition --as agent:copilot --server $server`
    Then the output has:
      | 0.dataset          | scene-set |
      | 0.revision         | 1 |
      | 0.repository       | kr://scene/object |
      | 0.search.state     | ready |
      | 0.schemas.0.objectId | schema/metric.definition |
    Then the output includes:
      | 0.schemas.0.fields[].access   | [filter text] |
      | 0.search.declarations[].status | ready |
    When I run `kc operations access-spec describe --dataset scene-set --revision 1 --as agent:copilot --server $server`
    Then the output has:
      | setId              | scene-set |
      | definitionRevision | 1 |
    Then the output includes:
      | specs[].repository | kr://scene/object |
    When I run `kc search --dataset scene-set --revision 1 --query merchandise --as agent:copilot --server $server`
    Then 1 hit metric/gmv
    Then the output has:
      | dataset.id       | scene-set |
      | dataset.revision | 1 |
    When I run `kc writer put urn:kc:object:metric/gmv#/definition --repo kr://scene/object --command-id gmv-unit-usd --value '{"name":"Gross merchandise value","expression":"SUM(l_extendedprice * (1 - l_discount))","unit":"USD","measureKey":"unique-measure-token-zz9"}'`
    Then the output has:
      | disposition      | APPLIED |
      | result.newCommit | nonempty |
    When I run `kc dataset define --dataset scene-set --revision 2 --source kr://scene/object=refs/heads/main@metrics/metric/gmv@metrics/metric/gmv --source kr://scene/graph=refs/heads/main@relations/rel@relations/rel --source kr://scene/object=refs/heads/main@schemas/metric@_schemas --source kr://scene/graph=refs/heads/main@schemas/relations@_schemas --source kr://scene/object=refs/heads/main@indexes/metric@indexes/metric --source kr://scene/graph=refs/heads/main@indexes/relation@indexes/relation`
    Then the output has:
      | setId    | scene-set |
      | revision | 2 |
    When I run `kc read urn:kc:object:metric/gmv#/definition --dataset scene-set --revision 1 --as agent:copilot --server $server`
    Then the output has:
      | objectId   | metric/gmv |
      | value.unit | CNY |
    When I run `kc read urn:kc:object:metric/gmv#/definition --dataset scene-set --revision 2`
    Then the output has:
      | objectId   | metric/gmv |
      | value.unit | USD |
    When I run `kc read urn:kc:object:metric/gmv#/definition --dataset scene-set --revision 1`
    Then the output has:
      | objectId   | metric/gmv |
      | value.unit | CNY |
    When I run `kc relations --dataset scene-set --revision 1 --object metric/gmv --as agent:copilot --server $server`
    Then the output includes:
      | hits[].objectId | rel/defines/gmv |
    When I run `kc read urn:kc:object:metric/gmv#/definition --dataset scene-set --revision 9 --as agent:copilot --server $server`
    Then error VERSION_UNRESOLVED
    When I run `kc read urn:kc:object:metric/gmv#/definition --repo kr://scene/object --revision 1 --as agent:copilot --server $server`
    Then error USAGE_INVALID
