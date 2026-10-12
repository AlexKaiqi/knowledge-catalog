# 从已接入仓出发，授权和写入仅属于本用例；不交付独立授权状态。

Feature: 授予写权后指定主体可写，其他主体写入仍被拒绝

  Scenario: 授予写权后指定主体可写，其他主体写入仍被拒绝
    When I run `kc grant add --principal writer-bot --action writer.commit --repo kr://scene/knowledge`
    Then the output has:
      | principal | writer-bot |
      | repo      | kr://scene/knowledge |
      | actions.0 | writer.commit |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | writer-bot |
      | rules[].repo      | kr://scene/knowledge |
      | rules[].actions.0 | writer.commit |

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
