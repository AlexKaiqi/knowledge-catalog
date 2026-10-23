# 在 object-published 上：目录零差异发布返回已接受版本，重试复用原回执，改动意图被拒。

Feature: probe zero diff directory

  Scenario: an unchanged directory keeps the accepted version and its original intent
    When I run `kc writer commit --command-id zero-diff-first --repo kr://scene/object --dir $materials/drafts`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.changed      | true |
      | result.newCommit    | nonempty |
    And I remember result.newCommit as accepted-commit
    When I run `kc read urn:kc:object:note/versioned`
    Then the output has:
      | objectId   | note/versioned |
      | value.text | first |
      | commit     | $accepted-commit |
    When I run `kc writer commit --command-id zero-diff-equal --repo kr://scene/object --dir $materials/drafts`
    Then the output has:
      | disposition      | APPLIED |
      | result.changed   | false |
      | result.oldCommit | $accepted-commit |
      | result.newCommit | $accepted-commit |
    When I run `kc writer commit --command-id zero-diff-equal --repo kr://scene/object --dir $materials/drafts`
    Then the output has:
      | disposition      | REPLAYED |
      | result.changed   | false |
      | result.newCommit | $accepted-commit |
    When I run `kc writer receipt --command-id zero-diff-equal`
    Then the output has:
      | receipt.result.changed   | false |
      | receipt.result.newCommit | $accepted-commit |
    When I run `kc writer commit --command-id zero-diff-equal --repo kr://scene/object --dir $materials/drafts-changed`
    Then error IDEMPOTENCY_CONFLICT
    When I run `kc writer head --repo kr://scene/object`
    Then the output has:
      | commit | $accepted-commit |
