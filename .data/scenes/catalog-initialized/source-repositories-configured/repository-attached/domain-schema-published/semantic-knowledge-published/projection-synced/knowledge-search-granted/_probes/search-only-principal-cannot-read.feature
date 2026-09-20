Feature: 只授仓搜权允许定位但拒绝正文读取

  @P-22 @KC-AGENT-01
  Scenario: 只授仓搜权允许定位但拒绝正文读取
    When I run `kc read --as searcher --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
    When I run `kc search --as searcher --repo kr://scene/knowledge --query merchandise`
    Then 1 hit metric/gmv
