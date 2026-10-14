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

    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:note%2Fx --as other --command-id x --value '{"v":1}'`
    Then error FORBIDDEN
    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:note%2Fy --as writer-bot --command-id y --value '{"v":1}'`
    Then the output has:
      | disposition           | APPLIED |
      | result.repositoryId   | kr://scene/knowledge |
      | result.newCommit      | nonempty |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fknowledge:note%2Fy`
    Then the output has:
      | objectId | note/y |
      | value.v             | 1 |
