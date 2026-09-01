# 部署：Taihu 身份认证接入

## 凭证

| 字段 | 值 |
|------|------|
| client_id | `knowledge-catalog` |
| client_secret | 从 Secret Manager 注入为 `KC_SERVICE_CLIENT_SECRET`；禁止写入仓库、镜像或日志 |
| 服务域名 | `test.dw-knowledge-base.tianqiong.woa.com` |
| OAuth2 授权服务器 | `http://iam.it.woa.com`（HTTP，非 HTTPS） |

如果真实凭据曾进入仓库或构建日志，删除文本并不能使它失效；必须在身份系统中撤销并轮换。

## 启动方式

### 方案 A：太湖网关后（推荐）

部署在太湖网关后面，网关自动校验 Bearer token 并注入 `x-tai-identity` header。

```bash
kc serve --auth taihu --listen :7380 \
  --service-client-id "knowledge-catalog" \
  --service-client-secret "$KC_SERVICE_CLIENT_SECRET"
```

### 方案 B：直连 introspection（不经网关）

```bash
kc serve --auth taihu --auth-url http://iam.it.woa.com --listen :7380 \
  --service-client-id "knowledge-catalog" \
  --service-client-secret "$KC_SERVICE_CLIENT_SECRET"
```

## 太湖平台配置

1. 登录 [tai.it.woa.com](https://tai.it.woa.com)
2. 创建应用（已创建：`knowledge-catalog`）
3. 在应用详情页配置登录可信域名：`test.dw-knowledge-base.tianqiong.woa.com`
4. 创建站点，关联域名 `test.dw-knowledge-base.tianqiong.woa.com`
5. 确认网关已注入 `x-tai-identity` header

## 验证

```bash
# 服务端健康检查
curl http://localhost:7380/readyz

# 带 token 访问 whoami
curl -H "Authorization: Bearer <access_token>" http://localhost:7380/identity/v1/whoami

# 客户端登录
kc login --server http://localhost:7380
# 浏览器打开 → 授权 → 另一终端
kc login --wait --server http://localhost:7380
```
