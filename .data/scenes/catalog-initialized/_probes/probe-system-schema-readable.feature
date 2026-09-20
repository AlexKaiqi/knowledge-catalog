Feature: System Schema discovery and read

  Scenario: initialized System Repository exposes its schemas and README
    When I run `kc show`
    Then the output has:
      | catalogId         | kr://scene/catalog |
      | repositories.1.id | absent |
    Then the output includes:
      | repositories[].id | kr://kc/system |
    When I run `kc schema list --repo kr://kc/system`
    Then the output has:
      | repository   | kr://kc/system |
      | continuation | absent |
    Then the output includes:
      | schemas[].objectId | schema/meta/schema-definition/v1 |
      | schemas[].objectId | schema/core/resource-descriptor/v1 |
      | schemas[].objectId | schema/core/relation/v1 |
      | schemas[].objectId | schema/core/readme/v1 |
      | schemas[].entity   | SchemaDefinition |
      | schemas[].entity   | ResourceDescriptor |
      | schemas[].entity   | Relation |
      | schemas[].entity   | Readme |
      | schemas[].description | 仓的自描述 Markdown；frontmatter 承载 Address，正文可搜。 |
    When I run `kc read --repo kr://kc/system --object schema/meta/schema-definition/v1`
    Then the output has:
      | objectId | schema/meta/schema-definition/v1 |
      | repository          | kr://kc/system |
      | value.entity        | SchemaDefinition |
      | value.pattern       | record |
    When I run `kc read --repo kr://kc/system --object schema/core/readme/v1`
    Then the output has:
      | objectId | schema/core/readme/v1 |
      | value.entity        | Readme |
      | value.aspect        | readme |
    When I run `kc read --repo kr://kc/system --object kc/system --aspect readme`
    Then the output has:
      | objectId            | kc/system |
      | aspectName          | readme |
      | value.body          | nonempty |
