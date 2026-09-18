# schema-read-granted：schema-reader 持有 knowledge.schema.read。
# 主体不用 bot：其它分叉会给 bot 叠加权。

Feature: schema-read-granted

  Scenario: construct
    When I run `kc grant add --principal schema-reader --action knowledge.schema.read --repo kr://scene/knowledge`
    Then the output has:
      | principal | schema-reader |
      | repo      | kr://scene/knowledge |
      | actions.0 | knowledge.schema.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | schema-reader |
      | rules[].actions.0 | knowledge.schema.read |
    When I run `kc schema list --as schema-reader --repo kr://scene/knowledge`
    Then the output has:
      | repository   | kr://scene/knowledge |
      | continuation | absent |
    Then the output includes:
      | schemas[].objectId | schema/metric.definition |
      | schemas[].entity      | Metric |
      | schemas[].description | 业务指标的定义。 |
