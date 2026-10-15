# review-branch-written：工作分支含改动；main 不动；read 仍旧值。

Feature: review-branch-written

  Scenario: construct
    When I run `kc writer branch --repo kr://scene/object --name review/scene`
    Then the output has:
      | repositoryId | kr://scene/object |
      | ref          | refs/heads/review/scene |
      | commitId     | nonempty |
    When I run `kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id scene-review --ref refs/heads/review/scene --value '{"text":"proposed"}'`
    Then the output has:
      | disposition      | APPLIED |
      | result.targetRef | refs/heads/review/scene |
      | result.newCommit | nonempty |
    When I run `kc read urn:kc:object:note/hello`
    Then the output has:
      | value.text | hi |
    When I run `kc read urn:kc:object:note/hello --ref refs/heads/review/scene`
    Then the output has:
      | value.text | proposed |
