# 在 knowledge-read-granted 上：授读后看见 Canonical（含未编进索引的字段）。

Feature: probe canonical visible

  @P-22 @KC-AGENT-01
  Scenario: read returns canonical
    """
    Agent as searcher (search+read)
    你现在有 knowledge.read。请再搜 merchandise 并读取 metric/gmv，
    确认能看到公式和未编进索引的 measureKey。
    """

    When I run `kc search --as searcher --repo kr://scene/knowledge --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc read --as searcher --repo kr://scene/knowledge --object metric/gmv`
    Then READ body is full canonical
