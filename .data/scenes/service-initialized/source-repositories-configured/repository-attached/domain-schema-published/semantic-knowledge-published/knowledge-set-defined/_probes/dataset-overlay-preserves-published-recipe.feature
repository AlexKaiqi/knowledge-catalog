Feature: 个人配方合成不发布或改变共享 Dataset

  Scenario: 个人配方合成不发布或改变共享 Dataset
    When I run `kc dataset overlay --file $materials/dataset-recipe.yaml --overlay $materials/dataset-overlay.yaml`
    Then the output has:
      | setId | personal-task |
      | sources.0.repository | kr://scene/graph |
      | sources.0.path | relations |
      | sources.0.subPath | relations/rel |
      | sources.1.repository | kr://scene/knowledge |
      | sources.1.path | references |
      | sources.1.subPath | metrics/metric/gmv |
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-set |
      | datasets[].revision | 1 |
      | repositories[].id | kr://scene/knowledge |
      | repositories[].id | kr://scene/graph |
    When I run `kc read --dataset personal-task --object metric/gmv`
    Then error KNOWLEDGE_SET_INVALID
