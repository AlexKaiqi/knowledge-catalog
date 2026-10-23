# 在 object-published 上：历史首页给游标，HEAD 推进后仅用游标续页仍按原依据。

Feature: probe history continuation

  Scenario: a cursor page keeps its original basis after the head advances
    When I run `kc writer head --repo kr://scene/object`
    Then the output has:
      | commit | nonempty |
    And I remember commit as base-commit
    When I run `kc log --repo kr://scene/object --object note/hello --limit 1`
    Then the output has:
      | logs.0.commit             | $base-commit |
      | logs.0.revisions.0.commit | $base-commit |
      | logs.0.revisions.0.status | RESOLVED |
      | complete                  | false |
      | continuation              | nonempty |
    And I remember continuation as base-cursor
    When I run `kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id history-first --value '{"text":"one"}'`
    Then the output has:
      | disposition      | APPLIED |
      | result.changed   | true |
      | result.newCommit | nonempty |
    And I remember result.newCommit as first-commit
    When I run `kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id history-second --value '{"text":"two"}'`
    Then the output has:
      | disposition      | APPLIED |
      | result.changed   | true |
      | result.newCommit | nonempty |
    And I remember result.newCommit as second-commit
    When I run `kc log --repo kr://scene/object --object note/hello --limit 1`
    Then the output has:
      | logs.0.commit             | $second-commit |
      | logs.0.revisions.0.commit | $second-commit |
      | complete                  | false |
      | continuation              | nonempty |
    And I remember continuation as first-cursor
    When I run `kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id history-third --value '{"text":"three"}'`
    Then the output has:
      | disposition      | APPLIED |
      | result.newCommit | nonempty |
    And I remember result.newCommit as third-commit
    When I run `kc log --limit 1 --continuation $first-cursor`
    Then the output has:
      | logs.0.commit             | $second-commit |
      | logs.0.revisions.0.commit | $first-commit |
      | continuation              | nonempty |
    When I run `kc log --limit 1 --continuation $first-cursor --commit $third-commit`
    Then error USAGE_INVALID
    When I run `kc log --repo kr://scene/object --object note/hello --limit 1`
    Then the output has:
      | logs.0.commit             | $third-commit |
      | logs.0.revisions.0.commit | $third-commit |
