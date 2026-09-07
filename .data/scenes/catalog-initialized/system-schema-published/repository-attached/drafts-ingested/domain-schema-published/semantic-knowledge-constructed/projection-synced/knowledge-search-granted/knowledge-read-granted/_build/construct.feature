# knowledge-read-granted：bot 在 search 之外又有 knowledge.read。

Feature: knowledge-read-granted

  Scenario: construct
    When I run `kc admin grant add --principal bot --action knowledge.read --repo kr://scene/knowledge`
    Then the output has:
      | principal | bot |
      | repo      | kr://scene/knowledge |
      | actions.0 | knowledge.read |
    When I run `kc admin grant list`
    Then the output includes:
      | rules[].principal | bot |
      | rules[].actions.0 | knowledge.read |
