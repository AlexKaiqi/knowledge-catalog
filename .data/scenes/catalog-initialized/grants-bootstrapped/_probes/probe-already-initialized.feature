# 在 grants-bootstrapped 上：已有任何 rule 即不能再 bootstrap。

Feature: probe already initialized

  Scenario: second bootstrap fails
    When I run `kc local grant bootstrap --principal agent:other`
    Then error PRECONDITION_FAILED
