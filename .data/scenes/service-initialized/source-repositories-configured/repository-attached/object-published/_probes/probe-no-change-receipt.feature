# 在 object-published 上：同值条件写返回无变化回执，重放与换内容各自可辨。

Feature: probe no change receipt

  Scenario: an equal value keeps the accepted commit and replays its own receipt
    When I run `kc read urn:kc:object:note/hello`
    Then the output has:
      | value.text | hi |
      | commit     | nonempty |
    And I remember commit as head-commit
    When I run `kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id no-change-equal --value '{"text":"hi"}'`
    Then the output has:
      | disposition      | APPLIED |
      | result.changed   | false |
      | result.oldCommit | $head-commit |
      | result.newCommit | $head-commit |
    When I run `kc writer receipt --command-id no-change-equal`
    Then the output has:
      | receipt.disposition      | APPLIED |
      | receipt.result.changed   | false |
      | receipt.result.newCommit | $head-commit |
    When I run `kc writer head --repo kr://scene/object`
    Then the output has:
      | commit | $head-commit |
    When I run `kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id no-change-equal --value '{"text":"different"}'`
    Then error IDEMPOTENCY_CONFLICT
    When I run `kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id no-change-moved --value '{"text":"different"}'`
    Then the output has:
      | disposition      | APPLIED |
      | result.changed   | true |
      | result.oldCommit | $head-commit |
      | result.newCommit | nonempty |
    When I run `kc writer receipt --command-id no-change-equal`
    Then the output has:
      | receipt.result.changed   | false |
      | receipt.result.newCommit | $head-commit |
