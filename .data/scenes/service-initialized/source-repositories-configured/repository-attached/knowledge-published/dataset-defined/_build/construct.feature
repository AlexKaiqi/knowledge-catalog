# dataset-defined：普通知识已发表后定义命名知识集 scene-notes。
# Define 冻结 selector→commit。消费走 --dataset。

Feature: dataset-defined

  Scenario: construct
    When I run `kc dataset define --dataset scene-notes --revision 1 --source kr://scene/knowledge=refs/heads/main`
    Then the output has:
      | setId | scene-notes |
      | revision    | 1 |
      | sources.0.repository | kr://scene/knowledge |
      | sources.0.selector | refs/heads/main |
      | sources.0.commit | nonempty |
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-notes |
      | datasets[].revision | 1 |
      | repositories[].id | kr://scene/knowledge |
    When I run `kc read --dataset scene-notes --object note/hello`
    Then the output includes:
      | [].objectId | note/hello |
      | [].repository | kr://scene/knowledge |
      | [].commit | nonempty |
