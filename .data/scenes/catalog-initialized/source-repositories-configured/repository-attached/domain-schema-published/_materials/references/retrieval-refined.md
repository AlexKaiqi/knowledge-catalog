# retrieval-refined：迁移后的参考说明

此文件保留原说明，不是状态、可执行 scene 或验证证据。实际 Oracle 由宿主 `_meta.yaml` 中的具名 Go 测试或独立 probe 承接。

# retrieval-refined

SEARCH 已形成固定候选窗后，`search:rerank` 与 `operations feedback record` 在同一 pin 上精炼。Provider 不能生成知识。反馈是晚于 Agent 请求的独立调用。

构建与探：`TestHTTPSearchRerankPreservesRetrievalEvidenceAndUsesOneFixedView`、`TestRerankEvidenceFeedbackAndTrainingSampleJourney`。
