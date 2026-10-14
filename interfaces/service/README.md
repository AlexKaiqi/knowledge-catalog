# Interface 调用任务

`Open` 接收固定声明范围、实际 Registry 和可信 RequestContext；`Service.Call` 解析实现、验证输入和效果授权、隔离 handler 视图、调用真实处理器并验证结果，返回固定合同与 Handler 依据。写效果需要显式授权口，注册不发权。基础合同仍由 interfaces 根拥有；本包不导入 LiveAccess 或来源协议。
