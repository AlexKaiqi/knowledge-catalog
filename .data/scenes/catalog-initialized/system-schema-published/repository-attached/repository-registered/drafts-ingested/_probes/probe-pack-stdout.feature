# 在 drafts-ingested 上：不带 --out 时 ChangeSet 在 stdout。仍未发表。

Feature: probe pack stdout

  Scenario: changeset on stdout
    When I run `kc pack --repo kr://scene/knowledge --dir $materials/drafts`
    Then the output has:
      | changeSet.targetRepository | kr://scene/knowledge |
    Then the output includes:
      | changeSet.operations[].address.objectId | schema/metric.definition |
    When I run `kc knowledge schema list --repo kr://scene/knowledge`
    Then the output has:
      | repository | kr://scene/knowledge |
      | schemas    | [] |
