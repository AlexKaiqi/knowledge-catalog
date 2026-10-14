# review-branch-written

`writer branch` 开出 review/scene，`writer put --ref` 把 proposed 写到分支；main 仍是 hi，分支写入不通知索引与 Dataset。审批由业务完成，KC 只在合并时做条件快进与对象校验。

两个用例各自从本前态开始：一条把分支合并进 main 并读回 proposed；另一条先让 main 被别的写入推进，再证明合并以 NON_FAST_FORWARD 拒绝。源分支在审阅后移动（CANDIDATE_MOVED）、重放、前置 Hook 否决与授权派生由本节点点名的 Go 证据承接。
