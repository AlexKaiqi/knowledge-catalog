# 在 schema-read-granted 上：可浏览 Schema，不可读实例。

Feature: probe browse without instance

  Scenario: schema is not body
    When I run `kc knowledge schema list --as bot --repo kr://scene/knowledge`
    Then the output has:
      | repository | kr://scene/knowledge |
      | exhausted  | true |
    Then the output includes:
      | schemas[].objectId | schema/metric.definition |
    When I run `kc knowledge read --as bot --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
