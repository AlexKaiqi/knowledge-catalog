Feature: Dataset 管理权不放行文件或仓级读搜

  Scenario: Dataset 管理权不放行文件或仓级读搜
    When I run `kc read --as manager --dataset scene-set --object metric/gmv`
    Then error FORBIDDEN
    When I run `kc search --as manager --repo kr://scene/knowledge --query merchandise`
    Then error FORBIDDEN
    When I run `kc read --as manager --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
