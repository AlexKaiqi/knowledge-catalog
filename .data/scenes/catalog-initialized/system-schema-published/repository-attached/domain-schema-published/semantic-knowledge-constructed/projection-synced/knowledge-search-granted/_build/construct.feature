# knowledge-search-granted：searcher 已有 knowledge.search，尚无 knowledge.read。
# 主体不用 bot：其它分叉会给 bot 叠加 knowledge.read，投递链屏蔽正文的探会假绿。

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
