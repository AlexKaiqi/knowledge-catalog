# dsh-loom

DSH 的 Knowledge Catalog 宿主集成。插件不注册模型工具；Agent 使用分组后的
`kc` CLI，文件访问使用 DSH 原有 shell 与文件系统工具。

## 运行合同

宿主必须注入：

- `KC_HOME`：绝对路径，保存私有任务上下文；
- `KC_CATALOG`、`KC_WORKSPACE`：任务要使用的组合；
- `KC_AS`：明确的 Agent principal；
- `KCFS_BIN`：可选，默认 `kcfs`。

任务创建时，根插件同步调用 `kcfs daemon-mount`。该命令只在所有知识目录均已
只读挂载并产生固定 pin 后返回；失败会阻止任务进入未挂载状态。上下文和 mount
manifest 写入 `$KC_HOME/tasks/`，不会写进用户项目。子任务复用父任务的同一 mount
和 pin；最后一个引用释放时调用 `kcfs stop` 卸载。

同一项目根若已有不同 Workspace，会明确拒绝。没有 FUSE 的环境返回能力错误，
不会把知识静默复制到项目。用户项目的普通文件仍可写，只有 Workspace 配方指定
的知识目录是只读 mount。

## Agent 使用

典型路径：

```bash
kc knowledge search --query '支付告警'
kc knowledge read --object 'runbook/payment-alert'
kc knowledge provenance --object 'runbook/payment-alert'
rg '回滚' knowledge/
```

Catalog、Workspace、pin 和身份由当前 mount 上下文继承；与上下文冲突的显式参数
必须被 CLI 拒绝。系统没有公开 Knowledge 枚举，也不会在检索能力缺失时改做全仓扫描。

## Files 面板

可选 Files 面板只读取 `$KC_HOME/tasks/*/context.json` 允许的已挂载目录，并通过
普通宿主文件 API 预览内容。它不调用 KC 文件协议，不接触 Repository 凭证，也不
向模型注册文件工具。开关偏好保存在 `$KC_HOME/ui/`。

## 开发验证

```bash
npm ci
npm run typecheck
npm test
npm run build
npm run pack:check
```

Linux FUSE 验收使用仓库根 `scripts/e2e-kcfs-linux.sh`。macOS 可运行类型、单元和
`kcfs plan` 检查，但实际 mount 应明确列为环境性跳过。
