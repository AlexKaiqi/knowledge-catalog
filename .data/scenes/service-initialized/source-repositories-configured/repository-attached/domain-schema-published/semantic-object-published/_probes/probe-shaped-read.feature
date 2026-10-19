# 在 semantic-object-published 上：按 GraphQL 形状只取 Metric definition 的 name 与 unit。

Feature: probe shaped read

  Scenario: read only the selected part of a published object
    When I run `kc read urn:kc:object:metric/gmv --shape '{ definition { name unit } lineage }'`
    Then the output has:
      | objectId                 | metric/gmv |
      | data.definition.name     | Gross merchandise value |
      | data.definition.unit     | CNY |
      | data.definition.expression | absent |
      | data.definition.measureKey | absent |
      | errors.0.path.0          | lineage |
      | errors.0.code            | OBJECT_REF_UNRESOLVED |
      | errors.1                 | absent |
      | sources.0.repository     | kr://scene/object |
      | sources.0.commit         | nonempty |
      | sources.0.aspects.0      | definition |
      | value                    | absent |
