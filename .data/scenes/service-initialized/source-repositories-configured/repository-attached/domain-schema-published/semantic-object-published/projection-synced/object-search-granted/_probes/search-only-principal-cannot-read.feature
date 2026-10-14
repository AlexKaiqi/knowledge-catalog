Feature: 只授仓搜权允许定位但拒绝正文读取

  @P-22 @KC-AGENT-01
  Scenario: 只授仓搜权允许定位但拒绝正文读取
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv --as searcher`
    Then error FORBIDDEN
    When I run `kc search --as searcher --repo kr://scene/object --query merchandise`
    Then 1 hit metric/gmv
