Feature: 同一主体经 Server 发现并检索回读 Dataset 知识

  Scenario: 同一主体经 Server 发现并检索回读 Dataset 知识
    Given local HTTP server
    When I run `kc catalog list --as agent:copilot --server $server`
    Then the output includes:
      | catalogs[].id | kr://scene/catalog |
    When I run `kc show --as agent:copilot --server $server`
    Then the output includes:
      | datasets[].id | scene-set |
    When I run `kc schema list --repo kr://scene/knowledge --as agent:copilot --server $server`
    Then the output has:
      | repository | kr://scene/knowledge |
    When I run `kc search --as agent:copilot --dataset scene-set --query merchandise --server $server`
    Then 1 hit metric/gmv
    When I run `kc read --as agent:copilot --dataset scene-set --object metric/gmv --aspect definition --server $server`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].value.name          | Gross merchandise value |
      | [].commit | nonempty |
    When I run `kc read --as agent:copilot --dataset scene-set --object rel/defines/gmv --server $server`
    Then the output includes:
      | [].objectId | rel/defines/gmv |
      | [].repository          | kr://scene/graph |
      | [].value.relationType  | defines |
    When I run `kc relations --as agent:copilot --dataset scene-set --object kc://scene/knowledge/metric/gmv --server $server`
    Then the output includes:
      | hits[].objectId | rel/defines/gmv |
    When I run `kc resolve --as agent:copilot --dataset scene-set --object metric/gmv --server $server`
    Then the output includes:
      | [].status | RESOLVED |
