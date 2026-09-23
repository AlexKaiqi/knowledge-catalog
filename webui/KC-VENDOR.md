# KC Dataset Web Console — vendored lakeFS webui

本目录是把 [treeverse/lakeFS](https://github.com/treeverse/lakeFS) 开源的 `webui/`
前端（Apache-2.0）vendor 进 KC 后改造出来的 **Dataset 管理前端**。它不是 lakeFS 的
管理界面，只连接 KC Server 的闭合 typed API 面。

## Pin 记录

- 上游仓库：`github.com/treeverse/lakeFS`
- 上游 tag：`v1.58.0`（子目录 `webui/`）
- 抽取时间：2026-09-22
- 上游许可：Apache-2.0。上游仓库根的 `LICENSE` 与 `NOTICE` 原样保留在本目录
  （`LICENSE`、`NOTICE`）；改造部分沿用同一许可发布。KC 对上游代码的修改都在
  `webui/src/extendable/lakefsApp.tsx`、`webui/src/pages/`（lakeFS 页面原样保留、
  不参与构建路由）与 `webui/src/lib/api/`、`webui/vite.config.mts`、
  `webui/package.json`、`webui/index.html`。

## KC 改了什么

- `src/lib/api/kc.ts`：KC API 适配层。所有调用只走 KC 闭合路由
  （`/identity/v1`、`/catalog/v1`、`/dataset-files/v1`、`/knowledge/v1`），
  错误按 KC FaultJSON（`code`/`message`/`details`）解析；凭证从
  `localStorage`（`kc.ui.token` / `kc.ui.as`）读取，分别对应 Server 的
  `Authorization` 与 `--auth local` 的 `X-Kc-As`。
- `src/lib/api/kcTypes.ts`：KC 侧形状（Dataset、版本、交付树条目、来源）。
- `src/extendable/kcApp.tsx` 与 `src/pages/kc/*`：KC 的路由与页面
  （Dataset 列表、版本、交付目录树浏览、发布表单）。
- `src/main.tsx`：入口改为 KC app。上游 `lakefsApp.tsx` 与 lakeFS 页面保留
  在 vendor 树中作为参考，不接入路由。
- `vite.config.mts`：构建产物输出到 `../cli/webapp`（`go:embed` 只能嵌包内
  路径），`base: '/ui/'`。
- `package.json`：包名改为 `kc-dataset-ui`，依赖集与上游一致，未新增。

## 构建

```bash
cd webui
npm ci        # 或 npm install
npm run build # 产物落在 ../cli/webapp
```

`kc serve` 启动后浏览器访问 `/ui/`。构建产物目录 `cli/webapp/` 是生成物：
`.gitignore` 忽略其内容，只保留 `.gitkeep` 占位（`go:embed` 要求目录存在；
`emptyOutDir` 会清掉占位文件，属预期）。前端尚未构建时，`/ui/` 会显示构建提示页。
