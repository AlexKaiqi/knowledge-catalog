Feature: Dataset file.read 可读清单但不授予仓级读搜权限

  Scenario: Dataset file.read 可读清单但不授予仓级读搜权限
    When I run `kc grant add --principal consumer --action file.read --dataset scene-set`
    Then the output has:
      | principal | consumer |
      | catalog   | absent |
      | dataset | scene-set |
      | actions.0 | file.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | consumer |
      | rules[].dataset | scene-set |
      | rules[].actions.0 | file.read |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv --as consumer --dataset scene-set`
    Then the output has:
      | objectId | metric/gmv |
      | commit | nonempty |
    When I run `kc search --as consumer --repo kr://scene/object --query merchandise`
    Then error FORBIDDEN
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv --as consumer`
    Then error FORBIDDEN
