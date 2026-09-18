# 在 dataset-retired 上：已退役的 Dataset 不再可消费。

Feature: probe retired

  Scenario: retired knowledge set
    When I run `kc read --dataset scene-retire-probe --object metric/gmv`
    Then error KNOWLEDGE_SET_INVALID
