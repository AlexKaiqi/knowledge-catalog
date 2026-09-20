Feature: 退役独立 Dataset 后其清单不可再读取

  Scenario: 退役独立 Dataset 后其清单不可再读取
    When I run `kc dataset define --dataset scene-retire-probe --revision 1 --source kr://scene/knowledge=refs/heads/main@metrics/metric/gmv@metrics/metric/gmv --source kr://scene/graph=refs/heads/main@relations/rel@relations/rel`
    Then the output has:
      | setId | scene-retire-probe |
      | revision    | 1 |
    When I run `kc dataset retire --dataset scene-retire-probe`
    Then the output has:
      | dataset | scene-retire-probe |
      | retired   | true |
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-retire-probe |
      | datasets[].retired     | true |
    When I run `kc read --dataset scene-retire-probe --object metric/gmv`
    Then error KNOWLEDGE_SET_INVALID
