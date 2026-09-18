# 在 system-schema-published 上：本机已配置知识仓，Catalog 尚未承认该源；
# 因此不能 define Dataset。接入仍由后续 attach 完成。

Feature: probe not registered

  Scenario: catalog does not admit an unregistered source
    When I run `kc show`
    Then the output has:
      | catalogId         | kr://scene/catalog |
      | repositories.1.id | absent |
    Then the output includes:
      | repositories[].id | kr://kc/system |
    When I run `kc dataset define --dataset premature --revision 1 --source kr://scene/knowledge=refs/heads/main`
    Then error KNOWLEDGE_SET_INVALID
