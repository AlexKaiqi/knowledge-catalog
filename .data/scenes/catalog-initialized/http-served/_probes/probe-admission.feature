# 在 http-served 上：admission show 是本人 grants + 外部申请入口，不是申请队列。
# 测试夹具尚未 bootstrap，administrators 为空。现场 `deployment init` 会把
# bootstrapPrincipal 列入 request.administrators（见 grants-bootstrapped）。

Feature: probe admission

  Scenario: caller sees own grants and no request queue
    When I run `kc admission show --as reader --server $server`
    Then the output has:
      | principal              | reader |
      | grants                 | [] |
      | request.url            | absent |
      | request.administrators | [] |
      | status                 | absent |
      | eligible               | absent |
      | currentActions         | absent |
