Feature: 独立授予仓读权后 SEARCH 与 READ 返回 Canonical

  @P-22 @KC-AGENT-01
  Scenario: 独立授予仓读权后 SEARCH 与 READ 返回 Canonical
    When I run `kc grant add --principal searcher --action knowledge.read --repo kr://scene/knowledge`
    Then the output has:
      | principal | searcher |
      | repo      | kr://scene/knowledge |
      | actions.0 | knowledge.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | searcher |
      | rules[].actions.0 | knowledge.read |
    """
    Agent as searcher (search+read)
    你现在有 knowledge.read。请再搜 merchandise 并读取 metric/gmv，
    确认能看到公式和未编进索引的 measureKey。
    """

    When I run `kc search --as searcher --repo kr://scene/knowledge --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc read --as searcher --repo kr://scene/knowledge --object metric/gmv`
    Then READ body is full canonical
