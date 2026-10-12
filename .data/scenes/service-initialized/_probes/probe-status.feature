# 在 service-initialized 上：库存列表可见。测试夹具的 allow 为空（现场
# `deployment init` 会写入 bootstrap-deployment-admin，见 grants-bootstrapped）。

Feature: probe empty registry

  Scenario: list without grants
    When I run `kc show`
    Then the output has:
      | catalogId | absent |
    When I run `kc grant list`
    Then the output has:
      | rules | [] |
