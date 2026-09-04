# 在 http-served 上：Client 凭据配对走 login / whoami / logout，不打开 --home。

Feature: probe cli pairing

  Scenario: local login then logout
    When I run `kc login --server $server --mode local --as user:reader`
    Then the output has:
      | status    | authenticated |
      | principal | user:reader |
      | mode      | local |
    When I run `kc whoami --server $server`
    Then the output has:
      | principal | user:reader |
    When I run `kc logout --server $server`
    Then the output has:
      | status | logged out |
