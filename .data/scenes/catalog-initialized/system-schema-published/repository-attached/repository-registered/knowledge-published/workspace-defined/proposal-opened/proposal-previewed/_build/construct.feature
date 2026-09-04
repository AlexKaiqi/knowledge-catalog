# proposal-previewed：proposal overlay 到知识集 pin 上。不是 pack 的 ChangeSet。

Feature: proposal-previewed

  Scenario: construct
    When I run `kc governance preview create --proposal PR-scene --workspace scene-notes`
    Then the output has:
      | previewId | nonempty |
    When I run `kc knowledge read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | value.text | hi |
