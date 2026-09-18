# named-repositories-created：两个托管仓已供给并登记。仓 ID = Graveler 名。

Feature: named-repositories-created

  Scenario: construct
    When I run `kc create --name table-meta --store lakefs`
    Then the output has:
      | repositoryId | table-meta |
      | name         | table-meta |
      | store        | lakefs |
    When I run `kc create --name sales-semantic --store lakefs`
    Then the output has:
      | repositoryId | sales-semantic |
      | name         | sales-semantic |
      | store        | lakefs |
    When I run `kc attach --repo table-meta`
    Then the output has:
      | repositoryId | table-meta |
    When I run `kc attach --repo sales-semantic`
    Then the output has:
      | repositoryId | sales-semantic |
    When I run `kc show`
    Then the output includes:
      | repositories[].id | kr://kc/system |
      | repositories[].id | table-meta |
      | repositories[].id | sales-semantic |
