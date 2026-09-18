# knowledge-read-granted：searcher 在 search 之外又有 knowledge.read。

Feature: knowledge-read-granted

  Scenario: construct
    When I run `kc grant add --principal searcher --action knowledge.read --repo kr://scene/knowledge`
    Then the output has:
      | principal | searcher |
      | repo      | kr://scene/knowledge |
      | actions.0 | knowledge.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | searcher |
      | rules[].actions.0 | knowledge.read |
