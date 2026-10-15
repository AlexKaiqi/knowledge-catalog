# 在 source-repositories-configured 上：普通 principal 不能写平台仓。

Feature: probe system immutable

  Scenario: writer cannot mutate system
    When I run `kc writer put urn:kc:object:schema/evil --repo kr://kc/system --as bot --command-id mutate-system --value '{"entity":"Evil"}'`
    Then error FORBIDDEN
