# retrieval-refined

SEARCH 已形成固定候选窗后，`search:rerank` 与 `operations feedback record` 在同一 pin 上精炼。Provider 不能生成知识。反馈是晚于 Agent 请求的独立调用。

构建与探：`TestHTTPSearchRerankPreservesRetrievalEvidenceAndUsesOneFixedView`、`TestRerankEvidenceFeedbackAndTrainingSampleJourney`。
