# writer-granted：bot 持有 writer.commit。

Feature: writer-granted

  Scenario: construct
    When I run `kc admin grant add --principal bot --action writer.commit --repo kr://scene/knowledge`
    Then the output has:
      | principal | bot |
      | repo      | kr://scene/knowledge |
      | actions.0 | writer.commit |
    When I run `kc admin grant list`
    Then the output includes:
      | rules[].principal | bot |
      | rules[].repo      | kr://scene/knowledge |
      | rules[].actions.0 | writer.commit |
