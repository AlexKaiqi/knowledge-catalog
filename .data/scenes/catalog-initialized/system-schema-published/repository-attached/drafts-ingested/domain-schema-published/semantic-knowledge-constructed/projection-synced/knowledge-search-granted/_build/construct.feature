# knowledge-search-granted：bot 已有 knowledge.search，尚无 knowledge.read。

Feature: knowledge-search-granted

  Scenario: construct
    When I run `kc admin grant add --principal bot --action knowledge.search --repo kr://scene/knowledge`
    Then the output has:
      | principal | bot |
      | repo      | kr://scene/knowledge |
      | actions.0 | knowledge.search |
    When I run `kc admin grant list`
    Then the output includes:
      | rules[].principal | bot |
      | rules[].actions.0 | knowledge.search |
