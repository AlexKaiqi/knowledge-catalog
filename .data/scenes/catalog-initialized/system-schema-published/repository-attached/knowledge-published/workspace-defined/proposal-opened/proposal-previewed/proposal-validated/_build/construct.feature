# proposal-validated：对该 Preview 做协议结构检查。不跑业务套件。

Feature: proposal-validated

  Scenario: construct
    When I run `kc governance preview validate --preview $previewId`
    Then the output has:
      | reportId | nonempty |
      | outcome  | PASSED |
    When I run `kc knowledge read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | value.text | hi |
