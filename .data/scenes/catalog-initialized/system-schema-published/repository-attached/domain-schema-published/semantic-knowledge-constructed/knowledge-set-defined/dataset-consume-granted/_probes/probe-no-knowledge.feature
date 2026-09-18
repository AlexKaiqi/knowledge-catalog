# 在 dataset-consume-granted 上：Dataset file.read 不放行仓 knowledge.*。

Feature: probe no knowledge

  Scenario: dataset file.read is not repository knowledge
    When I run `kc search --as consumer --repo kr://scene/knowledge --query merchandise`
    Then error FORBIDDEN
    When I run `kc read --as consumer --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
