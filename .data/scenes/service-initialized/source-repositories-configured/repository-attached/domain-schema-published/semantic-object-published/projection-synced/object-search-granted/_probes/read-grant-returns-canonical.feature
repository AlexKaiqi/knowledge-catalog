Feature: 独立授予仓读权后 SEARCH 与 READ 返回 Canonical

  @P-22 @KC-AGENT-01
  Scenario: 独立授予仓读权后 SEARCH 与 READ 返回 Canonical
    When I run `kc grant add --principal searcher --action object.read --repo kr://scene/object`
    Then the output has:
      | principal | searcher |
      | repo      | kr://scene/object |
      | actions.0 | object.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | searcher |
      | rules[].actions.0 | object.read |
    """
    Agent as searcher (search+read)
    你现在有 object.read。请再搜 merchandise 并读取 metric/gmv，
    确认能看到公式和未编进索引的 measureKey。
    """

    When I run `kc search --as searcher --repo kr://scene/object --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv --as searcher`
    Then READ body is full canonical
