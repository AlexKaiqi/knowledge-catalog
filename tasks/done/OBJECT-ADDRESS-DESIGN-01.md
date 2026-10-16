# OBJECT-ADDRESS-DESIGN-01 · 统一对象寻址设计（2026-10-13）

- [x] 设计文档完成：已补清对象身份、Repo／Dataset 访问语境、固定依据、JSON Pointer 与维护 Address 的职责；明确同名对象、Dataset 版本约束、对象写入与交付发布边界，计划同步完成观察。
- 范围与出口：仅 Guideline、计划、文档图与本条；不改代码、公开形状、测试、格式、权限或运行配置，不启动服务、安装依赖或提交。逻辑寻址规则本轮明确，具体 API 形状在公开合同落实；独立复核、owner／图重读、文档和质量门禁通过后只标记设计文档完成。
- 验证：独立复核通过，已重读相关 owner 及新增 Dataset／术语图关系；`make check-docs`（33 文档／171 关系）、`make quality`（退出 0，日志 `/tmp/kc-object-address-design-quality-20261013.log`）和本轮空白检查通过。范围不完整须如实说明，未新增整对象拒绝政策、第二写面或字段 PATCH；新增统一引用与读取计划尚待代码及真实入口证据。
