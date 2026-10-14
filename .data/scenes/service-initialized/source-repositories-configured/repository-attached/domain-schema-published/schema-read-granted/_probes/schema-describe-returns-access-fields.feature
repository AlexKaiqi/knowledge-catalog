Feature: Schema 内省返回字段路径并保持响应边界

  Scenario: Schema 内省返回字段路径并保持响应边界
    When I run `kc schema describe --as schema-reader --repo kr://scene/object --object schema/metric.definition`
    Then the output has:
      | repository              | kr://scene/object |
      | schemas.0.objectId      | schema/metric.definition |
      | schemas.0.fields        | nonempty |
      | schemas.0.fields.0.path | expression |
      | schemas.0.entity        | absent |
