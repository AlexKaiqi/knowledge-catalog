# 结构检查独立使用本节点冻结的 Preview；报告只属于本用例，不供另一个 probe 使用。

Feature: 执行 Preview 结构检查得到通过报告且主分支保持原值

  Scenario: 执行 Preview 结构检查得到通过报告且主分支保持原值
    When I run `kc governance preview validate --preview $previewId`
    Then the output has:
      | reportId | nonempty |
      | outcome  | PASSED |
    When I run `kc read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | value.text | hi |
