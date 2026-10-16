# semantic-dataset-defined：从两仓各取指定路径发布 scene-set（指标实体、defines 关系、Schema 及指标的索引声明）。
# 索引声明只在选进 Dataset 后才对该 Dataset 的检索生效。
# Define 冻结 selector→commit。消费走 --dataset，回执带 commit；不要先造 pin 文件。

Feature: semantic-dataset-defined

  Scenario: construct
    When I run `kc dataset define --dataset scene-set --revision 1 --source kr://scene/object=refs/heads/main@metrics/metric/gmv@metrics/metric/gmv --source kr://scene/graph=refs/heads/main@relations/rel@relations/rel --source kr://scene/object=refs/heads/main@schemas/metric@_schemas --source kr://scene/graph=refs/heads/main@schemas/relations@_schemas --source kr://scene/object=refs/heads/main@indexes/metric@indexes/metric --source kr://scene/graph=refs/heads/main@indexes/relation@indexes/relation`
    Then the output has:
      | setId | scene-set |
      | revision    | 1 |
      | sources.0.repository | kr://scene/object |
      | sources.0.selector | refs/heads/main |
      | sources.0.subPath | metrics/metric/gmv |
      | sources.0.commit | nonempty |
      | sources.1.repository | kr://scene/graph |
      | sources.1.selector | refs/heads/main |
      | sources.1.subPath | relations/rel |
      | sources.1.commit | nonempty |
      | sources.2.subPath | _schemas |
      | sources.2.commit | nonempty |
      | sources.3.subPath | _schemas |
      | sources.3.commit | nonempty |
      | sources.4.subPath | indexes/metric |
      | sources.4.commit | nonempty |
      | sources.5.subPath | indexes/relation |
      | sources.5.commit | nonempty |
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-set |
      | datasets[].revision | 1 |
      | repositories[].id | kr://scene/object |
      | repositories[].id | kr://scene/graph |
    When I run `kc read urn:kc:object:metric/gmv --dataset scene-set`
    Then the output has:
      | objectId | metric/gmv |
      | repository | kr://scene/object |
      | commit | nonempty |
