Feature: 仓提交前进后 Dataset 仍可读取已发表指标

  Scenario: 仓提交前进后 Dataset 仍可读取已发表指标
    When I run `kc read --dataset scene-set --object metric/gmv`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].commit | nonempty |
    When I run `kc writer put --command-id after-pin --repo kr://scene/knowledge --object note/after-pin --value '{"text":"after"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc read --dataset scene-set --object metric/gmv`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].commit | nonempty |
    When I run `kc read --repo kr://scene/knowledge --object note/after-pin`
    Then the output has:
      | objectId | note/after-pin |
      | value.text | after |
