Feature: Dataset file.read 可读清单但不授予仓级读搜权限

  Scenario: Dataset file.read 可读清单但不授予仓级读搜权限
    When I run `kc grant add --principal consumer --action file.read --catalog kr://scene/catalog --dataset scene-set`
    Then the output has:
      | principal | consumer |
      | catalog   | kr://scene/catalog |
      | dataset | scene-set |
      | actions.0 | file.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | consumer |
      | rules[].dataset | scene-set |
      | rules[].actions.0 | file.read |
    When I run `kc read --as consumer --dataset scene-set --object metric/gmv`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].commit | nonempty |
    When I run `kc search --as consumer --repo kr://scene/knowledge --query merchandise`
    Then error FORBIDDEN
    When I run `kc read --as consumer --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
