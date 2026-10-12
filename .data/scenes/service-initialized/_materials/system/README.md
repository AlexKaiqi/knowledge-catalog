# System Schema 旅程夹具

跟踪源仍是 `knowledge/system/_schemas/`（随版本交付的出版源，部署经 Writer 发布到实际 system 仓）。
本目录给人看接入前要读的协议 Schema。字段 access 是唯一的索引声明书写位置：
值结构与查询能力共用版本化声明，结构校验、查询合同解释和投影就绪分别维护。
部署夹具经 Writer 发布完整系统出版物，construct 和 probe 只读取 `kr://kc/system`，
不通过这些材料重写运行时只读的系统仓。
文件必须与跟踪源字节一致（`TestSceneSystemSchemaMaterialsMatchEmbed`）。
