# object-search-granted：searcher 已有 object.search，尚无 object.read。
# 主体不用 bot：使用独立主体明确只有搜权的前态。

Feature: object-search-granted

  Scenario: construct
    When I run `kc grant add --principal searcher --action object.search --repo kr://scene/object`
    Then the output has:
      | principal | searcher |
      | repo      | kr://scene/object |
      | actions.0 | object.search |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | searcher |
      | rules[].actions.0 | object.search |
