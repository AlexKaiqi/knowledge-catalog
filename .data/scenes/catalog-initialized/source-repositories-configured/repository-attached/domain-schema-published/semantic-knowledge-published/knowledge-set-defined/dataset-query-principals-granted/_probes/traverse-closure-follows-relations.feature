Feature: 有界邻域闭包沿关系到达邻居对象

  Scenario: traverse 在 Dataset 固定范围内闭包一跳邻居
    Given local HTTP server
    When I run `kc traverse --as agent:copilot --dataset scene-set --object kc://scene/knowledge/metric/gmv --max-hops 1 --server $server`
    Then the output includes:
      | edges[].objectId | rel/defines/gmv |
      | nodes[].objectId | schema/metric.definition |
    Then the output has:
      | exhausted | true |
