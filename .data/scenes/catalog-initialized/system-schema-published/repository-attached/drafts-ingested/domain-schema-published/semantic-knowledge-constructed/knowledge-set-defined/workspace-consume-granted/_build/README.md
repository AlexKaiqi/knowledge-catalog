# workspace-consume-granted

持有命名知识集的 `workspace.consume`。进入组合面，不放行任何 `knowledge.*`（AUTH-02）。命名知识集 SEARCH 还要另授 `knowledge.search`；`catalog.read` 不能跳过 consume。

两成员时只授一仓 `knowledge.read`：精确 READ/RESOLVE/LOG/PROVENANCE/RELATIONS fail closed；SEARCH 两仓都进候选且 `complete`，无读权命中走交付链屏蔽正文。

构建与探：`TestWorkspaceConsumeDoesNotImplyKnowledgeActions`、`TestAuthorizeWorkspaceKnowledgeSeparatesConsumeFromSearch`、`TestWorkspaceAuthorizationCoverageIsHonest`。
