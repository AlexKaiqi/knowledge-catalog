# drafts-ingested：接入方把 Domain Schema 草稿预览成 ChangeSet。不发表。

Feature: drafts-ingested

  Scenario: construct
    When I run `kc writer ingest --repo kr://scene/knowledge --dir $materials/drafts --out $home/schema.changeset.json`
    Then the output has:
      | changeSet                    | absent |
      | out                          | nonempty |
      | diagnostics.files            | 1 |
      | diagnostics.schemaObjects    | 1 |
      | diagnostics.frontmatterIdentities | 1 |
      | diagnostics.warnings         | [] |
    Then the output includes:
      | files[].objectId | schema/metric.definition |
    When I run `kc knowledge read --repo kr://scene/knowledge --object schema/metric.definition`
    Then error KNOWLEDGE_REF_UNRESOLVED
    When I run `kc knowledge schema browse --repo kr://scene/knowledge`
    Then the output has:
      | repository | kr://scene/knowledge |
      | schemas    | [] |
