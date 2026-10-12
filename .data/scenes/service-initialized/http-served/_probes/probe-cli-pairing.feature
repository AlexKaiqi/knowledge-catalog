# 在 http-served 上：Client 凭据配对走 login / whoami / logout，不打开 --home。
# 现场 ttyd 若预置了 KC_AS，会盖住会话主体；本探不设 KC_AS。

Feature: probe cli pairing

  Scenario: local login then logout
    When I run `kc login --server $server --mode local --as reader`
    Then the output has:
      | status    | authenticated |
      | principal | reader |
      | mode      | local |
    When I run `kc whoami --server $server`
    Then the output has:
      | principal | reader |
    When I run `kc logout --server $server`
    Then the output has:
      | status | logged out |
