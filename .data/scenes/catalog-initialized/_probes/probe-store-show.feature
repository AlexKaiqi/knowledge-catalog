# 在 catalog-initialized 上：本机 adapter / DSN（无密钥）。不是库存。

Feature: probe store show

  Scenario: local store envelope
    When I run `kc local store show`
    Then the output has:
      | repository | dolt |
      | profile    | local |
