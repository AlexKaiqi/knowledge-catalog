# 在 schema-read-granted 上：describe 是 AccessHints 内省，不是实体目录，也不返回实例正文。

Feature: probe schema describe

  Scenario: describe published domain schema
    When I run `kc schema describe --as schema-reader --repo kr://scene/knowledge --object schema/metric.definition`
    Then the output has:
      | repository              | kr://scene/knowledge |
      | schemas.0.objectId      | schema/metric.definition |
      | schemas.0.fields        | nonempty |
      | schemas.0.fields.0.path | expression |
      | schemas.0.entity        | absent |
