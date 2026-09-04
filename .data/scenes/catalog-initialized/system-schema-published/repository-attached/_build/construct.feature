# repository-attached：接入方打开空知识仓。
# 本机 attach（⓪）之后再 Catalog 登记（①）。还没有 Domain Schema，也没有实例。

Feature: repository-attached

  Scenario: construct
    When I run `kc local repository attach --repo kr://scene/knowledge`
    Then the output has:
      | repositoryId | kr://scene/knowledge |
      | head         | nonempty |
    When I run `kc catalog repo register --repo kr://scene/knowledge`
    When I run `kc catalog show`
    Then the output has:
      | catalogId | kr://scene/catalog |
    Then the output includes:
      | repositories[].id | kr://scene/knowledge |
      | repositories[].id | kr://kc/system |
