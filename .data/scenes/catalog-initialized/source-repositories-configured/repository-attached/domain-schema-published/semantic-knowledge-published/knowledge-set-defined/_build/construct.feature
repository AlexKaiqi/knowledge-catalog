# knowledge-set-defined：从两仓各取指定路径发布 scene-set（指标实体 + defines 关系）。
# Define 冻结 selector→commit。消费走 --dataset，回执带 commit；不要先造 pin 文件。

Feature: knowledge-set-defined

  Scenario: construct
    When I run `kc dataset define --dataset scene-set --revision 1 --source kr://scene/knowledge=refs/heads/main@metrics/metric/gmv@metrics/metric/gmv --source kr://scene/graph=refs/heads/main@relations/rel@relations/rel`
    Then the output has:
      | setId | scene-set |
      | revision    | 1 |
      | sources.0.repository | kr://scene/knowledge |
      | sources.0.selector | refs/heads/main |
      | sources.0.subPath | metrics/metric/gmv |
      | sources.0.commit | nonempty |
      | sources.1.repository | kr://scene/graph |
      | sources.1.selector | refs/heads/main |
      | sources.1.subPath | relations/rel |
      | sources.1.commit | nonempty |
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-set |
      | datasets[].revision | 1 |
      | repositories[].id | kr://scene/knowledge |
      | repositories[].id | kr://scene/graph |
    When I run `kc read --dataset scene-set --object metric/gmv`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].repository | kr://scene/knowledge |
      | [].commit | nonempty |
