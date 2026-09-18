# catalog-audit-granted：auditor 持有 catalog.audit.read。
# 主体不用 bot：其它分叉会给 bot 叠加权。

Feature: catalog-audit-granted

  Scenario: construct
    When I run `kc grant add --principal auditor --action catalog.audit.read --catalog kr://scene/catalog`
    Then the output has:
      | id        | nonempty |
      | principal | auditor |
      | catalog   | kr://scene/catalog |
      | actions.0 | catalog.audit.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | auditor |
      | rules[].catalog   | kr://scene/catalog |
      | rules[].actions.0 | catalog.audit.read |
    When I run `kc catalog audit --as auditor`
    Then the output has:
      | source    | catalog |
      | catalogId | kr://scene/catalog |
      | entries   | nonempty |
