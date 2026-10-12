Feature: 仓 HEAD 前进后重新发布 Dataset 并回读新版

  Scenario: 仓 HEAD 前进后重新发布 Dataset 并回读新版
    When I run `kc writer put --command-id republish-head --repo kr://scene/knowledge --object note/republish --value '{"text":"republish"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc dataset define --dataset scene-set --revision 2 --source kr://scene/knowledge=refs/heads/main@metrics/metric/gmv@metrics/metric/gmv --source kr://scene/graph=refs/heads/main@relations/rel@relations/rel --source kr://scene/knowledge=refs/heads/main@schemas/metric@_schemas --source kr://scene/graph=refs/heads/main@schemas/relations@_schemas`
    Then the output has:
      | setId | scene-set |
      | revision    | 2 |
      | sources.0.repository | kr://scene/knowledge |
      | sources.0.subPath | metrics/metric/gmv |
      | sources.0.commit | nonempty |
      | sources.1.repository | kr://scene/graph |
      | sources.1.subPath | relations/rel |
      | sources.1.commit | nonempty |
    When I run `kc read --dataset scene-set --object metric/gmv`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].repository | kr://scene/knowledge |
      | [].commit | nonempty |
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-set |
      | datasets[].revision | 2 |
