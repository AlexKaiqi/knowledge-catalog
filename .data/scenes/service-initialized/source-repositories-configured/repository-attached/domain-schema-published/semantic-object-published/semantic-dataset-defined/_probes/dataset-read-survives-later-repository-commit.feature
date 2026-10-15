Feature: 仓提交前进后 Dataset 仍可读取已发表指标

  Scenario: 仓提交前进后 Dataset 仍可读取已发表指标
    When I run `kc read urn:kc:object:metric/gmv --dataset scene-set`
    Then the output has:
      | objectId | metric/gmv |
      | commit | nonempty |
    When I run `kc writer put urn:kc:object:note/after-pin --repo kr://scene/object --command-id after-pin --value '{"text":"after"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:metric/gmv --dataset scene-set`
    Then the output has:
      | objectId | metric/gmv |
      | commit | nonempty |
    When I run `kc read urn:kc:object:note/after-pin`
    Then the output has:
      | objectId | note/after-pin |
      | value.text | after |
