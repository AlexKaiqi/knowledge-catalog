# 在 principals-granted 上：请求如何绑定 principal。

Feature: probe identity

  @P-23 @KC-AGENT-01
  Scenario: local pairing binds the asserted principal
    """
    local HTTP 用 X-Kc-As 扮演身份，跳过真实 Taihu。
    无凭证不能登录。Agent 自报 onBehalfOf 应被拒绝。
    """

    When HTTP GET /identity/v1/whoami
    Then error UNAUTHENTICATED
    When HTTP GET /identity/v1/whoami as taihu:alice
    Then whoami is taihu:alice
    When HTTP GET /identity/v1/whoami as agent:copilot
    Then whoami is agent:copilot
    When HTTP GET /identity/v1/whoami as service:etl
    Then whoami is service:etl
    When HTTP GET /identity/v1/whoami as agent:copilot on-behalf-of taihu:alice
    Then error FORBIDDEN
