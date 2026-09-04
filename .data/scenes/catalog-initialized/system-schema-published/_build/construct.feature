# 接入方能读到 System Schema。夹具在 _materials/，与 knowledge/system/schemas 对账。
# init 已把 kr://kc/system 登记进 Catalog；本节点不经 Writer（平台仓不可写）。

Feature: system-schema-published

  Scenario: construct
    When I run `kc knowledge schema browse --repo kr://kc/system`
    Then the output has:
      | repository        | kr://kc/system |
      | exhausted         | true |
      | coverage.complete | true |
    Then the output includes:
      | schemas[].objectId | schema/meta/schema-definition/v1 |
      | schemas[].objectId | schema/core/resource-descriptor/v1 |
      | schemas[].objectId | schema/core/relation/v1 |
      | schemas[].objectId | schema/core/source-profile/v1 |
    When I run `kc knowledge read --repo kr://kc/system --object schema/meta/schema-definition/v1`
    Then the output has:
      | knowledgeRef.object | schema/meta/schema-definition/v1 |
      | repository          | kr://kc/system |
      | value.entity        | SchemaDefinition |
      | value.pattern       | record |
    When I run `kc knowledge read --repo kr://kc/system --object schema/core/source-profile/v1`
    Then the output has:
      | knowledgeRef.object | schema/core/source-profile/v1 |
      | value.entity        | SourceProfile |
      | value.pattern       | record |
