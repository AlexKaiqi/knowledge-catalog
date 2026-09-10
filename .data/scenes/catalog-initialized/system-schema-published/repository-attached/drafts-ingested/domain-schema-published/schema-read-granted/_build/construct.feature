# schema-read-granted：bot 持有 knowledge.schema.read。

Feature: schema-read-granted

  Scenario: construct
    When I run `kc grant add --principal bot --action knowledge.schema.read --repo kr://scene/knowledge`
    Then the output has:
      | principal | bot |
      | repo      | kr://scene/knowledge |
      | actions.0 | knowledge.schema.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | bot |
      | rules[].actions.0 | knowledge.schema.read |
    When I run `kc knowledge schema list --as bot --repo kr://scene/knowledge`
    Then the output has:
      | repository   | kr://scene/knowledge |
      | continuation | absent |
    Then the output includes:
      | schemas[].objectId | schema/metric.definition |
