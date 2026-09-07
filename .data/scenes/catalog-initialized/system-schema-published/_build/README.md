# System Schema 可读

部署 fixture 提供内置只读 System 信任根，本节点通过公开 Schema/READ 观察其内容；业务 Snapshot 由独立的 existing repository fixture 提供，尚未登记。显式远端发布只走 `kc deployment system publish --config`；正式部署测试另行验证该入口，Server 启动不补写 System Snapshot。
