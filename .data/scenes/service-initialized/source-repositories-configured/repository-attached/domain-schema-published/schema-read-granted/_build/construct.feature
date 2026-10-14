# schema-read-granted：schema-reader 持有 object.schema.read。
# 主体不用 bot：使用具名主体明确本用例的授权边界。

Feature: schema-read-granted

  Scenario: construct
    When I run `kc grant add --principal schema-reader --action object.schema.read --repo kr://scene/object`
    Then the output has:
      | principal | schema-reader |
      | repo      | kr://scene/object |
      | actions.0 | object.schema.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | schema-reader |
      | rules[].actions.0 | object.schema.read |
    When I run `kc schema list --as schema-reader --repo kr://scene/object`
    Then the output has:
      | repository   | kr://scene/object |
      | continuation | absent |
    Then the output includes:
      | schemas[].objectId | schema/metric.definition |
      | schemas[].entity      | Metric |
      | schemas[].description | 业务指标的定义。 |
