# proposal-previewed：proposal overlay 到本次解析的 Dataset 上。不是 Writer 提交用的 ChangeSet。
# Dataset 成员 commit 必须等于 proposal base（create 时的 target HEAD）。
# Define 把 selector 冻到当前 main：共享 Catalog 上其它分叉推进 HEAD 后，旧 vN 会 VALIDATION_BASIS_MISMATCH。

Feature: proposal-previewed

  Scenario: construct
    When I run `kc dataset define --dataset scene-notes --revision 2 --source kr://scene/knowledge=refs/heads/main`
    Then the output has:
      | setId | scene-notes |
      | revision    | 2 |
      | sources.0.repository | kr://scene/knowledge |
      | sources.0.commit | nonempty |
    When I run `kc governance preview create --proposal PR-scene --dataset scene-notes`
    Then the output has:
      | previewId           | nonempty |
      | candidate.commitId  | nonempty |
      | candidate.repositoryId | kr://scene/knowledge |
    When I run `kc read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | value.text | hi |
