# 客户端合成临时配方；共享 Workspace 与 Catalog 不因个人布局改变。
Feature: client dataset overlay

  Scenario: compose a personal recipe without publishing it
    When I run `kc dataset overlay --file $materials/dataset-recipe.yaml --overlay $materials/dataset-overlay.yaml`
    Then the output has:
      | setId | personal-task |
      | sources.0.repository | kr://scene/knowledge |
      | sources.0.path | references |
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-set |
      | datasets[].revision | 1 |
      | repositories[].id | kr://scene/knowledge |
      | repositories[].id | kr://scene/graph |
    When I run `kc read --dataset personal-task --object metric/gmv`
    Then error KNOWLEDGE_SET_INVALID
