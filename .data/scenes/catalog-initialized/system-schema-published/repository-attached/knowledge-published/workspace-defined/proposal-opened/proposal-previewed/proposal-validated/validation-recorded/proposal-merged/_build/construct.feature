# proposal-merged：清单齐则快进仓 Ref。construct 只 merge。

Feature: proposal-merged

  Scenario: construct
    When I run `kc governance proposal merge --proposal PR-scene --preview $previewId --validation $reportId`
    Then the output has:
      | proposalId | PR-scene |
      | commitId   | nonempty |
    When I run `kc knowledge read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | value.text | proposed |
