Feature: Dataset resolve 授权不放行文件与仓正文读取

  Scenario: Dataset resolve 授权不放行文件与仓正文读取
    When I run `kc grant add --principal resolver --action dataset.resolve --catalog kr://scene/catalog --dataset scene-set`
    Then the output has:
      | principal | resolver |
      | catalog   | kr://scene/catalog |
      | dataset | scene-set |
      | actions.0 | dataset.resolve |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | resolver |
      | rules[].dataset | scene-set |
      | rules[].actions.0 | dataset.resolve |
    When I run `kc read --as resolver --dataset scene-set --object metric/gmv`
    Then error FORBIDDEN
    When I run `kc read --as resolver --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
