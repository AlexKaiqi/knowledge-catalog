Feature: 同一主体经 Server 发现并检索回读 Dataset 对象

  Scenario: 同一主体经 Server 发现并检索回读 Dataset 对象
    Given local HTTP server
    When I run `kc show --as agent:copilot --server $server`
    Then the output has:
      | catalogId | absent |
    When I run `kc show --as agent:copilot --server $server`
    Then the output includes:
      | datasets[].id | scene-set |
    When I run `kc schema list --repo kr://scene/object --as agent:copilot --server $server`
    Then the output has:
      | repository | kr://scene/object |
    When I run `kc search --as agent:copilot --dataset scene-set --query merchandise --server $server`
    Then 1 hit metric/gmv
    When I run `kc read urn:kc:object:metric/gmv#/definition --as agent:copilot --dataset scene-set --server $server`
    Then the output has:
      | objectId | metric/gmv |
      | value.name          | Gross merchandise value |
      | commit | nonempty |
    When I run `kc read urn:kc:object:rel/defines/gmv --as agent:copilot --dataset scene-set --server $server`
    Then the output has:
      | objectId | rel/defines/gmv |
      | repository          | kr://scene/graph |
      | value.relationType  | defines |
    When I run `kc relations --as agent:copilot --dataset scene-set --object metric/gmv --server $server`
    Then the output includes:
      | hits[].objectId | rel/defines/gmv |
    When I run `kc resolve --as agent:copilot --dataset scene-set --object metric/gmv --server $server`
    Then the output includes:
      | [].status | RESOLVED |
