Feature: 管理权不跨 Dataset 且不授予发权能力

  Scenario: 管理权不跨 Dataset 且不授予发权能力
    When I run `kc dataset retire --as manager --dataset scene-other`
    Then error FORBIDDEN
    When I run `kc grant add --as manager --principal other --action file.read --catalog kr://scene/catalog --dataset scene-set`
    Then error FORBIDDEN
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-set |
