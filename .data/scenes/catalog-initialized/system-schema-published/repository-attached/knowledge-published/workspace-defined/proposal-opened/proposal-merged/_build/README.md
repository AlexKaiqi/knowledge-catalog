# proposal-merged

gate 证据齐全后 `governance proposal merge` 快进 main。下次 `read --workspace` 才见新值。candidate 再提交或 main 被推走分别是 `CANDIDATE_MOVED` / `NON_FAST_FORWARD`。

构建与探：`TestT9MergeDoesNotNeedPromote`、`TestT9MergeRejectsMovedMain`。
