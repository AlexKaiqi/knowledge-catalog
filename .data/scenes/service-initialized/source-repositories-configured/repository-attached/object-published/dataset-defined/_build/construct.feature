# dataset-defined：普通对象已发表后定义命名 Dataset scene-notes。
# Define 冻结 selector→commit。消费走 --dataset。

Feature: dataset-defined

  Scenario: construct
    When I run `kc dataset define --dataset scene-notes --revision 1 --source kr://scene/object=refs/heads/main`
    Then the output has:
      | setId | scene-notes |
      | revision    | 1 |
      | sources.0.repository | kr://scene/object |
      | sources.0.selector | refs/heads/main |
      | sources.0.commit | nonempty |
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-notes |
      | datasets[].revision | 1 |
      | repositories[].id | kr://scene/object |
    When I run `kc read urn:kc:object:note/hello --dataset scene-notes`
    Then the output has:
      | objectId | note/hello |
      | repository | kr://scene/object |
      | commit | nonempty |
