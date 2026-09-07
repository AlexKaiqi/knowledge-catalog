# 配置是显式部署输入；没有 config 时不能猜测本机 Home 或建立空部署。
Feature: deployment requires configuration

  Scenario: deployment management rejects missing configuration
    When I run `kc deployment init`
    Then error USAGE_INVALID
    When I run `kc deployment status`
    Then error USAGE_INVALID
    When I run `kc deployment system publish`
    Then error USAGE_INVALID
    When I run `kc catalog show`
    Then the output has:
      | catalogId | kr://scene/catalog |
      | workspaces | [] |
