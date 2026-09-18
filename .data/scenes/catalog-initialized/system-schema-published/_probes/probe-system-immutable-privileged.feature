# 在 system-schema-published 上：bootstrap * 仍不能写平台仓。
# 无 writer.commit 的 bot 在 probe-system-immutable 失败于授权，进不到
# “System Repository is immutable”。

Feature: probe system immutable privileged

  Scenario: privileged writer cannot mutate system
    When I run `kc writer put --command-id mutate-system-admin --repo kr://kc/system --object schema/evil --value '{"entity":"Evil"}'`
    Then error FORBIDDEN
