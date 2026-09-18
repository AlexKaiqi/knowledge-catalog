# 在 domain-schema-published 上：同一目录相对当前发布版本没有未提交的差。

Feature: probe diff

  Scenario: directory already matches HEAD
    When I run `kc diff --repo kr://scene/knowledge --dir $materials/drafts`
    Then the output has:
      | repository | kr://scene/knowledge |
      | commit     | nonempty |
      | changes    | [] |
