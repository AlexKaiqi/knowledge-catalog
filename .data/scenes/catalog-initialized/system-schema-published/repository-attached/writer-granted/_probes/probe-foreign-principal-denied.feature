# 在 writer-granted 上：他人写入失败关闭；writer-bot 可 PUT。

Feature: probe foreign principal denied

  Scenario: write isolation
    When I run `kc writer put --as other --command-id x --repo kr://scene/knowledge --object note/x --value '{"v":1}'`
    Then error FORBIDDEN
    When I run `kc writer put --as writer-bot --command-id y --repo kr://scene/knowledge --object note/y --value '{"v":1}'`
    Then the output has:
      | disposition           | APPLIED |
      | result.repositoryId   | kr://scene/knowledge |
      | result.newCommit      | nonempty |
    When I run `kc read --repo kr://scene/knowledge --object note/y`
    Then the output has:
      | objectId | note/y |
      | value.v             | 1 |
