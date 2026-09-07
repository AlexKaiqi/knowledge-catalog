# 在 repository-attached 上：本机已挂仓，库存仍不承认该源。

Feature: probe not registered

  Scenario: catalog does not admit an unregistered source
    When I run `kc catalog repo list`
    Then the output has:
      | catalogId         | kr://scene/catalog |
      | repositories.1.id | absent |
    Then the output includes:
      | repositories[].id | kr://kc/system |
    When I run `kc workspace define --workspace premature --revision 1 --source kr://scene/knowledge=refs/heads/main`
    Then error WORKSPACE_INVALID
