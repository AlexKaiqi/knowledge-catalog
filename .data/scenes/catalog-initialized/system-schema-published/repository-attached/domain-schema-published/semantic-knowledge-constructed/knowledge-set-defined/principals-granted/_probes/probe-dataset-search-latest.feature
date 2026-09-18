# 在 principals-granted 上：Dataset 检索跟已发布配方走。
# HEAD 前进后未再发版的 --dataset 仍钉发表时的 commit。
# 精确历史重放抄回执里的 --repo --commit。此探会改仓并再发一版，须排在 grant 探之前。

Feature: probe dataset search latest

  Scenario: dataset follows published recipe; exact replay uses commit
    When I run `kc search --dataset scene-set --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc read --repo kr://scene/knowledge --commit $last.hits.0.commit --object metric/gmv --aspect definition`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].value.name          | Gross merchandise value |
    When I run `kc writer put --command-id after-search --repo kr://scene/knowledge --object note/after-search --value '{"text":"after-search"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc operations projection sync --repo kr://scene/knowledge`
    Then the output has:
      | repository  | kr://scene/knowledge |
      | basisCommit | nonempty |
    When I run `kc read --dataset scene-set --object metric/gmv --aspect definition`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].value.name          | Gross merchandise value |
    When I run `kc dataset define --dataset scene-set --revision 2 --source kr://scene/knowledge=refs/heads/main@metrics/metric/gmv@metrics/metric/gmv --source kr://scene/graph=refs/heads/main@relations/rel@relations/rel`
    Then the output has:
      | setId | scene-set |
      | revision    | 2 |
      | sources.0.commit | nonempty |
    When I run `kc operations projection sync --repo kr://scene/knowledge`
    Then the output has:
      | repository  | kr://scene/knowledge |
      | basisCommit | nonempty |
    When I run `kc search --dataset scene-set --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc read --dataset scene-set --object metric/gmv --aspect definition`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].value.name          | Gross merchandise value |
