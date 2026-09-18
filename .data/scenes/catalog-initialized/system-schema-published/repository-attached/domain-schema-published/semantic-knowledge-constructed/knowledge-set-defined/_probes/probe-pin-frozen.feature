# 在 knowledge-set-defined 上：v1 的文件清单不随之后的仓提交移动。
# 此探会写入知识仓，执行器只在 home 副本上跑。

Feature: probe dataset frozen

  Scenario: later commit does not move published items
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
