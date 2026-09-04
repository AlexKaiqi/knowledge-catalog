# 在 system-schema-published 上：普通 principal 不能写平台仓。

Feature: probe system immutable

  Scenario: writer cannot mutate system
    When I run `kc writer put --as bot --command-id mutate-system --repo kr://kc/system --object schema/evil --value '{"entity":"Evil"}'`
    Then error FORBIDDEN
