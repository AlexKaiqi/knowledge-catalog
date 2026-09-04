# repository-attached：本机挂上空知识仓（H4）。Catalog 尚未承认该源。

Feature: repository-attached

  Scenario: construct
    When I run `kc local repository attach --repo kr://scene/knowledge`
    Then the output has:
      | repositoryId | kr://scene/knowledge |
      | head         | nonempty |
    When I run `kc local status`
    Then the output has:
      | catalog.repositoryId | kr://scene/catalog |
      | home                 | absent |
      | namespace            | absent |
    Then the output includes:
      | repos[].id | kr://scene/knowledge |
    When I run `kc catalog show`
    Then the output has:
      | catalogId         | kr://scene/catalog |
      | repositories.1.id | absent |
    Then the output includes:
      | repositories[].id | kr://kc/system |
