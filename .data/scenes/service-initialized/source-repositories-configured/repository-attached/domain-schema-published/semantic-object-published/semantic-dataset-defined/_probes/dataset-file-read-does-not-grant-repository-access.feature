Feature: Dataset file.read 可读清单但不授予来源仓读权限

  Scenario: Dataset file.read 可读清单但不授予来源仓读权限
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
    When I run `kc read urn:kc:object:metric/gmv --as consumer --dataset scene-set`
    Then the output has:
      | objectId | metric/gmv |
      | commit | nonempty |
    When I run `kc search --as consumer --dataset scene-set --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc read urn:kc:object:metric/gmv --as consumer`
    Then error FORBIDDEN
