# 在 schema-read-granted 上：可浏览 Schema，不可读实例。

Feature: probe browse without instance

  Scenario: schema is not body
    When I run `kc schema list --as schema-reader --repo kr://scene/knowledge`
    Then the output has:
      | repository   | kr://scene/knowledge |
      | continuation | absent |
    Then the output includes:
      | schemas[].objectId | schema/metric.definition |
      | schemas[].entity      | Metric |
      | schemas[].description | 业务指标的定义。 |
    When I run `kc read --as schema-reader --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
