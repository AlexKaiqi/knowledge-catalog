# 在 dataset-resolve-granted 上：dataset.resolve 不能读清单内文件，也不能走仓 knowledge.read。

Feature: probe resolve is not file.read

  Scenario: dataset.resolve is not consume
    When I run `kc read --as resolver --dataset scene-set --object metric/gmv`
    Then error FORBIDDEN
    When I run `kc read --as resolver --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
