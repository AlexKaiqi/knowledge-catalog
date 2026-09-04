# 在 schema-read-granted 上：describe 是 AccessHints 内省，不返回实例正文。

Feature: probe schema describe

  Scenario: describe published domain schema
    When I run `kc knowledge schema describe --as bot --repo kr://scene/knowledge --object schema/metric.definition`
    Then the output has:
      | repository | kr://scene/knowledge |
    Then the output includes:
      | schemas[].objectId | schema/metric.definition |
      | schemas[].entity   | Metric |
