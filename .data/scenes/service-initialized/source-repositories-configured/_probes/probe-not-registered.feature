# 在 source-repositories-configured 上：本机已配置仓，登记空间 尚未承认该源；
# 因此不能 define Dataset。接入仍由后续 attach 完成。

Feature: probe not registered

  Scenario: catalog does not admit an unregistered source
    When I run `kc show`
    Then the output has:
      | catalogId         | absent |
      | repositories.1.id | absent |
    Then the output includes:
      | repositories[].id | kr://kc/system |
    When I run `kc dataset define --dataset premature --revision 1 --source kr://scene/object=refs/heads/main`
    Then error DATASET_INVALID
