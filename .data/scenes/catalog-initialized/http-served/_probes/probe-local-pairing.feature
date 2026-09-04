# 在 http-served 上：local 配对；空凭证不是匿名访客。

Feature: probe local pairing

  Scenario: whoami binds asserted principal
    When HTTP GET /identity/v1/whoami
    Then error UNAUTHENTICATED
    When HTTP GET /identity/v1/whoami as user:reader
    Then whoami is user:reader
