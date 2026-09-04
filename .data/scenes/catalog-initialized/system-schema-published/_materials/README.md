# System Schema 旅程夹具

跟踪源仍是 `knowledge/system/schemas/`（Server 内置信任根，`go:embed`）。
本目录给人看接入前要读的协议 Schema；construct 只 READ `kr://kc/system`，不经 Writer。
文件必须与跟踪源字节一致（`TestSceneSystemSchemaMaterialsMatchEmbed`）。
