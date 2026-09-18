# grant-revoked

规则已 `grant remove`。下一次命令按当前 allow 求值；旧 pin 不能绕过撤权（KS-02）。本节点收回的是 `revoke-probe` 的 `catalog.read`，不是整份 allow。

构建与探：`TestUserJourneyManageAgentAccess`。
