# 在 semantic-object-published 上：形状只做投影；URN 已带定位符或形状超出投影时整次读取拒绝。

Feature: probe shaped read refusals

  Scenario: refuse shapes beyond projection and keep an open Schema field located
    When I run `kc read urn:kc:object:metric/gmv#/definition --shape '{ definition }'`
    Then error USAGE_INVALID
    When I run `kc read urn:kc:object:metric/gmv --shape '{ d: definition }'`
    Then error USAGE_INVALID
    When I run `kc read urn:kc:object:metric/gmv --shape '{ definition { name { first } } }'`
    Then error USAGE_INVALID
    When I run `kc read urn:kc:object:metric/gmv --shape '{ definition { name owner } }'`
    Then the output has:
      | data.definition.name | Gross merchandise value |
      | errors.0.path.1      | owner |
      | errors.0.code        | OBJECT_REF_UNRESOLVED |
      | errors.1             | absent |
