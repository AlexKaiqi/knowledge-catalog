# validation-recorded：只绑定外部套件已给出的 PASSED。不执行检查。

Feature: validation-recorded

  Scenario: construct
    When I run `kc governance validation record --preview $previewId --suite scene-contract --outcome PASSED`
    Then the output has:
      | reportId | nonempty |
      | outcome  | PASSED |
    When I run `kc knowledge read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | value.text | hi |
