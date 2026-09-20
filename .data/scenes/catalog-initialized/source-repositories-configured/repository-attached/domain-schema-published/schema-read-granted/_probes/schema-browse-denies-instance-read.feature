Feature: Schema 读权允许目录浏览但拒绝实例正文

  Scenario: Schema 读权允许目录浏览但拒绝实例正文
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
