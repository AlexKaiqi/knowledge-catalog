# DOC-14 · 原生 Dolt 与 Tree 能力隔离

以下保留当时的能力隔离问题与资格标准；Dolt adapter 已退役，涉及它的现状、路径和处理范围不是当前活动要求。未来 native provider 的资格仍由现行 Provider 合同拥有。

- [x] 收口 native Knowledge adapter 的公开能力边界。所属阶段：M5 的基础纠偏，方向已选定，可独立推进。

**问题与影响：** 规模设计要求 native Knowledge adapter 不暴露 TreeStore；当前 object/dolt 仍实现 TreeStore、DirectoryReader 并转发 ApplyTreeCommit。调用方仍可能获得原始路径写入口，削弱“知识写入必须经过 Writer”的能力隔离。它是确定的设计/实现差距，不等于已证明生产请求发生了绕过。

**处理范围：** 先盘点真实调用者、文件投影与现有 Conformance 的需求，再拆清 native 知识能力与通用 Snapshot Tree adapter。不能只删除接口让旧测试失效，也不能把通用 snapshot/dolt 一并退役。

**完成标准：** 原生知识服务装配不暴露 raw tree 写旁路；Reader/Writer、关系检索、批量/精确读取和恢复仍走正式合同；必要的文件能力由明确的适配层承担；增加能力拒绝/毒化反例，并运行保留的 provider 合同。

依据：[规模设计](../../doc/engineering/testing.md) §4、原 Dolt 接口与转发（历史实现已随 adapter 退役删除）、[MVP 缺口](../../doc/engineering/testing.md)。
