Feature: Schema 内省只返回结构，检索能力看索引声明

  Scenario: Schema 内省只返回字段结构，不带检索能力
    When I run `kc schema describe --as schema-reader --repo kr://scene/object --object schema/metric.definition`
    Then the output has:
      | repository              | kr://scene/object |
      | schemas.0.objectId      | schema/metric.definition |
      | schemas.0.fields        | nonempty |
      | schemas.0.fields.0.path | expression |
      | schemas.0.fields.0.access | absent |
      | schemas.0.entity        | absent |
