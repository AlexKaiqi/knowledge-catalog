# 在 dataset-manage-granted 上：dataset.manage 不放行消费或仓知识。

Feature: probe no consume

  Scenario: dataset.manage is not file.read or knowledge
    When I run `kc read --as manager --dataset scene-set --object metric/gmv`
    Then error FORBIDDEN
    When I run `kc search --as manager --repo kr://scene/knowledge --query merchandise`
    Then error FORBIDDEN
    When I run `kc read --as manager --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
