# 在 knowledge-search-granted 上：无 knowledge.read 时 SEARCH 屏蔽正文、READ fail closed。

Feature: probe delivery stripped

  @P-22 @KC-AGENT-01
  Scenario: search locates, read stays closed
    When I run `kc read --as searcher --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
    When I run `kc search --as searcher --repo kr://scene/knowledge --query merchandise`
    Then 1 hit metric/gmv
