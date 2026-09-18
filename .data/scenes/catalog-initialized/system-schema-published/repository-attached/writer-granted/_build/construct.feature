# writer-granted：writer-bot 持有 writer.commit。
# 主体不用 bot：其它分叉会给 bot 叠加权。

Feature: writer-granted

  Scenario: construct
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
