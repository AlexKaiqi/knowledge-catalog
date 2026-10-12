# 在 service-initialized 上：登记表自己的 git 历史。不是对象 log。

Feature: probe registry audit

  Scenario: registry history after init
    When I run `kc registry audit`
    Then the output has:
      | source    | registry |
      | catalogId | absent |
      | entries   | nonempty |
