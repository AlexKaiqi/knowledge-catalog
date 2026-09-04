# 在 catalog-initialized 上：登记表自己的 git 历史。不是对象 log。

Feature: probe catalog audit

  Scenario: registry history after init
    When I run `kc catalog audit`
    Then the output has:
      | source    | catalog |
      | catalogId | kr://scene/catalog |
      | entries   | nonempty |
