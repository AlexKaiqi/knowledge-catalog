# permissions-aspect-published

仓内 `permissions` Aspect 已 COMMIT。它是外部授权快照（SOURCE 知识），不是 `kc knowledge read` 闸门，也不能放行 SELECT，更不写入 `allow.json`。

构建与探：`TestUserJourneyKnowledgeGrantDoesNotAuthorizeAccess`。
