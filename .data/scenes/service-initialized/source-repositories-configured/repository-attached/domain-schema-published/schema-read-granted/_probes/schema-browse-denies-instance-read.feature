Feature: Schema 读权允许目录浏览但拒绝实例正文

  Scenario: Schema 读权允许目录浏览但拒绝实例正文
    When I run `kc schema list --as schema-reader --repo kr://scene/object`
    Then the output has:
      | repository   | kr://scene/object |
      | continuation | absent |
    Then the output includes:
      | schemas[].objectId | schema/metric.definition |
      | schemas[].entity      | Metric |
      | schemas[].description | 业务指标的定义。 |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv --as schema-reader`
    Then error FORBIDDEN
