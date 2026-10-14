Feature: 仓提交前进后 Dataset 仍可读取已发表指标

  Scenario: 仓提交前进后 Dataset 仍可读取已发表指标
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv --dataset scene-set`
    Then the output has:
      | objectId | metric/gmv |
      | commit | nonempty |
    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fobject:note%2Fafter-pin --command-id after-pin --value '{"text":"after"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv --dataset scene-set`
    Then the output has:
      | objectId | metric/gmv |
      | commit | nonempty |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:note%2Fafter-pin`
    Then the output has:
      | objectId | note/after-pin |
      | value.text | after |
