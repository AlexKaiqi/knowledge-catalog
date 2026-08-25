# internal/journal/

本机过程账与幂等 stamp 的 JSONL 机制，不是 Catalog 历史、知识 LOG 或外部事件流。

| 文件 | 负责 |
|---|---|
| `journal.go` | 单文件事件 append/read |
| `multi.go` | 多账本组合读取 |
| `stamp.go` | 请求/操作 stamp |
