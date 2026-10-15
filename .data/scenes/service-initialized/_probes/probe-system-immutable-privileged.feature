# 在 source-repositories-configured 上：bootstrap * 仍不能写平台仓。
# 无 writer.commit 的 bot 在 probe-system-immutable 失败于授权，进不到
# “System Repository is immutable”。

Feature: probe system immutable privileged

  Scenario: privileged writer cannot mutate system
    When I run `kc writer put urn:kc:object:schema/evil --repo kr://kc/system --command-id mutate-system-admin --value '{"entity":"Evil"}'`
    Then error FORBIDDEN
