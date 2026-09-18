# 在 dataset-manage-granted 上：一份 Dataset 的 manage 不转到另一份，也不是发权。

Feature: probe manage scope

  Scenario: dataset.manage does not transfer or administer
    When I run `kc dataset retire --as manager --dataset scene-other`
    Then error FORBIDDEN
    When I run `kc grant add --as manager --principal other --action file.read --catalog kr://scene/catalog --dataset scene-set`
    Then error FORBIDDEN
    When I run `kc show`
    Then the output includes:
      | datasets[].id | scene-set |
