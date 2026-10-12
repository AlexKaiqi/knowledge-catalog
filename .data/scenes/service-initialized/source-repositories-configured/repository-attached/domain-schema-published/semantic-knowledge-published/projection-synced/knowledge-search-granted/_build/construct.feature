# knowledge-search-granted：searcher 已有 knowledge.search，尚无 knowledge.read。
# 主体不用 bot：使用独立主体明确只有搜权的前态。

Feature: knowledge-search-granted

  Scenario: construct
    When I run `kc grant add --principal searcher --action knowledge.search --repo kr://scene/knowledge`
    Then the output has:
      | principal | searcher |
      | repo      | kr://scene/knowledge |
      | actions.0 | knowledge.search |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | searcher |
      | rules[].actions.0 | knowledge.search |
