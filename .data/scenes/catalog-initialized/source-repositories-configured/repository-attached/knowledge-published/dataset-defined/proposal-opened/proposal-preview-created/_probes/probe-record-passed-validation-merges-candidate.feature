# 在同一用例中记录外部结果、读取未变化的主分支，再使用该报告合并并读回；不依赖结构检查 probe 的报告。

Feature: 记录外部通过报告后合并候选并从主分支读回新值

  Scenario: 记录外部通过报告后合并候选并从主分支读回新值
    When I run `kc governance validation record --preview $previewId --suite scene-contract --outcome PASSED`
    Then the output has:
      | reportId | nonempty |
      | outcome  | PASSED |
    When I run `kc read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | value.text | hi |

    When I run `kc governance proposal merge --proposal PR-scene --preview $previewId --validation $reportId`
    Then the output has:
      | proposalId | PR-scene |
      | commitId   | nonempty |
    When I run `kc read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | value.text | proposed |
