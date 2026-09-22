# 微信第三方平台管理平台

用**一个**微信开放平台「第三方平台（服务商）」账号，集中管理团队的数十个小程序：一份代码模板批量下发到多个小程序（每个小程序带自己的 `ext` 配置）、批量提审、批量发布，并把全流程的授权、额度、审核与发布结果留成台账。

**它解决什么问题**：以前要给每个小程序单独配上传密钥、单独上传、单独提审、单独发布，几十个账号就是几十倍重复劳动，且无法统一看进度。本平台把「一份模板 + 逐小程序变量」作为基本单位：

- 代码在**平台账号**侧统一维护（模板库），小程序侧只接收下发结果；
- **平台账号只需在开放平台后台配置一次**（授权事件 URL、加急/提审额度、IP 白名单、域名登记），**不再需要给每个小程序单独配置上传密钥**，也不需要在每个小程序后台逐个操作；
- 批量作业按 `appid` 串行下发，自动处理隐私检测等待、审核等待、额度耗尽暂停、失败分类重试与断点续跑。

> 面向「照着做」的接入操作手册见 `docs/onboarding.md`；故障处置见 `docs/runbook.md`；改代码前先读 `AGENTS.md`。

## 一、能力清单

| 能力 | 前端入口 | 关键接口（见 `api/openapi.yaml`） | 说明 |
|---|---|---|---|
| 小程序授权管理 | 小程序管理 | `GET /authorizers/authorization-url`、`POST /authorizers/authorize`、`POST /authorizers/sync`、`POST /authorizers/resync-tokens` | 生成 PC/H5 授权链接与二维码；授权回调自动登记；`authorized/updateauthorized/unauthorized` 事件自动登记与失效；令牌全量重扫 |
| 代码模板库 | 代码模板 | `GET /drafts`、`POST /drafts/{draftId}/add-to-template`、`GET /templates`、`PATCH /templates/{templateId}`、`DELETE /templates/{templateId}` | 草稿箱 ↔ 模板库同步、设为默认模板、备注、删除；**模板库上限 200**（`limit` 字段回传） |
| 批量上传代码 | 批量任务 → 新建 | `POST /jobs/preview`、`POST /jobs`（`type=commit`） | `ext_json` 变量渲染 + 逐应用预览；版本号模板；简单/高级 ext 两种模式 |
| 批量提审 | 批量任务 → 新建 | `POST /jobs`（`type=submit_audit`）；提审配置 `GET/POST/PATCH/DELETE /audit-profiles` | 提审配置（类目/标题/标签/UGC 声明/隐私声明）；类目字段自动校验；额度感知与暂停 |
| 批量发布 | 批量任务 → 新建 | `POST /jobs`（`type=release`）；单应用 `POST /releases/{appid}/release`、`/gray`、`/revert` | 全量发布、灰度发布（比例只能递增）、取消灰度、版本回退、体验版二维码 |
| 前置体检 | 前置体检 | `POST /preflight` | 授权状态、权限集 18、小程序资料、类目、隐私指引、域名、额度、模板库余量逐项结论 |
| 作业中心 | 批量任务 / 任务详情 | `GET /jobs`、`GET /jobs/{id}`、`GET /jobs/{id}/items`、`/start`、`/pause`、`/resume`、`/cancel`、`/retry-failed` | 暂停/恢复/取消/重试失败项/断点续跑；逐项请求与响应留档 |
| 审核与发布台账 | 审核管理 / 发布管理 | `GET /audits`、`POST /audits/sync`、`/undo`、`/speed-up`、`GET /audits/{appid}/quota`、`GET /releases` | 审核单状态对账（事件 + 轮询兜底）、撤回、加急、额度、发布记录 |
| 三类日志 | 日志 | `GET /logs/api-calls`、`GET /logs/callbacks`、`GET /logs/operations` | 微信调用（含 errcode/endpoint 过滤）、回调事件（解密明文）、平台操作 |
| 运行参数设置 | 设置 | `GET/PUT /platform/settings`、`GET /platform/status` | 并发、最大尝试次数、QPS、日志保留天数、默认模板/提审配置、超时与等待上限 |
| 单小程序运维 | 小程序管理 → 详情 | `/apps/{appid}/visit-status`、`/pages`、`/support-version`、`/apps/domains/apply`、`/releases/{appid}/trial-qrcode`、`/releases/{appid}/version` | 服务状态开关、页面列表、基础库版本与用户占比、域名批量配置、体验版二维码与版本信息 |

契约共 **52 个路径 / 62 个 operationId**（`grep -c "^  /" api/openapi.yaml` 与 `grep -c operationId api/openapi.yaml`），上表未逐条列举的以 `api/openapi.yaml` 为准。微信回调 `/callback/component`、`/callback/message/:appid` **不在契约内**：它们收发 XML 且成功时必须返回纯文本 `success`，与 JSON 契约不兼容，在 `server/internal/router` 中单独注册。

## 二、技术栈

| 端 | 技术 |
|---|---|
| 后端 | Go + Gin + GORM + MySQL/MariaDB；JWT 鉴权；作业引擎在进程内运行 |
| 前端 | Vue 3 + TypeScript（strict，零 `any`）+ Vite + Naive UI + Vue Router |
| 契约 | `api/openapi.yaml`（OpenAPI 3.0）为前后端对齐的唯一依据 |
| 代码生成 | Go：`oapi-codegen`（types + gin）→ `server/internal/gen/api.gen.go`；TS：`openapi-typescript` → `web/src/api/schema.d.ts` |
| 端口 | 后端 `:8091`，前端开发服务器 `:5174`（Vite 将 `/api` 代理到后端） |
| 部署 | 容器镜像发布到 ghcr：`ghcr.io/sakana-1314/wx-open:server` / `:web`（详见 4.5） |
| CI/CD | GitHub Actions：`.github/workflows/ci-backend.yml`、`ci-frontend.yml`、`build-images.yml` |

## 三、目录结构

```
├── api/openapi.yaml          # 唯一接口契约（52 路径 / 62 操作）
├── db/init.sql               # 建库建用户（wx_platform / wx_platform_test）
├── deploy/                   # 部署样例：docker-compose.yml、systemd 单元、nginx 反代
├── .github/workflows/        # CI：后端检查 / 前端检查 / 镜像构建发布（ghcr）
├── server/
│   ├── Dockerfile            # 后端镜像（golang 构建 → alpine 运行，非 root）
│   ├── cmd/server/main.go    # 入口：加载 .env → 连库 → 迁移/种子 → 令牌 → 回调 → 作业引擎
│   └── internal/
│       ├── config/           # 环境变量 → 类型化配置（凭据可缺省，缺失不 panic）
│       ├── auth/             # JWT 签发校验、鉴权中间件、登录限流
│       ├── database/         # GORM 连接、AutoMigrate、平台状态与种子数据
│       ├── model/            # GORM 模型 + 强类型枚举 + 微信返回码处置表（errcodes.go）
│       ├── repo/             # 数据访问（未找到统一返回 repo.ErrNotFound）
│       ├── core/             # 公共地基：哨兵错误、WeChatError、目标选择、ext_json 渲染、
│       │                     #   授权链接、运行参数、平台状态、Repos 聚合、Env、TokenStore 适配
│       ├── wxcrypt/          # 微信消息加解密（PKCS#7 块大小 32、EncodingAESKey 轮换）
│       ├── wxapi/            # 微信 HTTP 客户端（分能力、错误分类、QPS 限流、空 body 发 {}）
│       ├── wxtoken/          # 票据与令牌管理（单飞刷新、提前 10 分钟、refresh_token 轮换、全量重拉）
│       ├── callback/         # 两个公开回调入口（先回 success 再异步处理）
│       ├── batch/            # 作业引擎（步骤门控、按 appid 串行、分类重试、waiting、断点续跑、单实例锁）
│       ├── wxauth/           # 业务：授权链路、授权方同步、选项、模板库、域名/页面/服务状态
│       ├── wxaudit/          # 业务：审核状态与对账、撤回/加急/额度、发布/灰度/回退、前置体检
│       ├── wxjob/            # 业务：作业创建/预览/控制 + 步骤执行器 + 引擎 Store 适配
│       ├── mockwx/           # 内置模拟微信服务端（无凭据端到端验证，45 个路由）
│       ├── secretbox/        # refresh_token 落库加密（AES-256-GCM）
│       ├── service/          # 组装：Container（唯一 handler 依赖入口）、Auth/Logs、回调业务分发
│       ├── handler/          # 实现 oapi-codegen 的 ServerInterface：只做参数绑定与响应写出
│       └── router/           # 路由、CORS、统一 JSON 错误、公开回调路由注册
└── web/
    ├── Dockerfile            # 前端镜像（pnpm build → nginx 托管 + 反代 /api、/callback）
    ├── nginx.conf            # 容器内 nginx 模板（BACKEND_HOST/BACKEND_PORT 经 envsubst 注入）
    └── src/
        ├── api/              # client.ts（openapi-fetch + Bearer）、schema.d.ts（生成，勿手改）
        ├── layouts/          # 侧栏（桌面 220/64px 可折叠；≤820px 汉堡 + 抽屉）
        ├── views/            # 概览 / 小程序管理 / 代码模板 / 前置体检 / 批量任务 / 任务详情 /
        │                     #   新建任务 / 审核管理 / 发布管理 / 日志 / 设置 / 授权回调 / 登录
        └── components/       # PageHeader / StatusTag 等
```

## 四、快速开始

### 4.1 初始化数据库

```bash
make db-init          # 等价于 mysql -u root < db/init.sql
```

会创建 `wx_platform`（主库）、`wx_platform_test`（集成测试库）与用户 `wxplatform`（默认密码 `wxplatform_dev_password`，生产必须改）。

### 4.2 配置后端

```bash
cp server/.env.example server/.env
```

最少需要填两个变量，**缺失时服务直接拒绝启动**：

| 变量 | 说明 |
|---|---|
| `JWT_SECRET` | 平台自身登录 JWT 签名密钥，建议 32 位以上随机串 |
| `ADMIN_PASSWORD` | `admin` 账号密码 |

**不填微信凭据（`WX_COMPONENT_*`）也能正常启动**：服务会打印一行警告，概览页显示「第三方平台凭据未配置」，批量/授权等真实微信调用会被拦截并返回明确提示。要真正调用微信接口，必须完成第五节的接入配置。

### 4.3 启动后端与前端

```bash
make dev-backend      # 后端 :8091（cd server && go run ./cmd/server）
make dev-web          # 前端 :5174（另开一个终端）
# 或一起启动：
make dev
```

浏览器打开 <http://127.0.0.1:5174>，用用户名 `admin` 与 `server/.env` 里的 `ADMIN_PASSWORD` 登录。
后端换端口时同步改 `web/vite.config.ts` 的 proxy target；`PUBLIC_BASE_URL` 决定回调地址与 `redirect_uri`，本地调试可保持默认 `http://127.0.0.1:8091`。

### 4.4 无凭据本地体验（推荐先走这条）

没有任何真实微信凭据时，用内置模拟微信服务端可以走通「授权 → 草稿 → 模板 → 批量上传 → 提审 → 发布」全流程：

```bash
MOCK_WX=1 make dev-backend
```

`MOCK_WX=1` 时：

- 内置 mock 服务端会在本机随机端口启动，`WX_API_BASE` 被自动改写到它（日志会打印「⚠️ MOCK_WX=1：已启用内置模拟微信服务端 …」），**不会调用真实微信**；
- 未配置 `WX_COMPONENT_*` 时自动使用 mock 凭据（`wx_mock_component` / `mock_verify_token` / 固定的 43 字符 AESKey），因此回调接入校验也走得通；
- `MOCK_WX_FAIL_RATE=0.2` 可注入 20% 随机失败，用于验证失败分类、退避重试与暂停逻辑。

mock 端到端回归测试（无需真实凭据）：

```bash
make e2e-mock          # 使用内置模拟微信服务端 + 独立测试库 wx_platform_e2e 跑完整链路
```

### 4.5 容器化部署（镜像由 GitHub Actions 发布）

后端与前端各有一个 Dockerfile，镜像发布在 ghcr，**tag 固定为组件名**：

| 镜像 | 内容 | 端口 |
|---|---|---|
| `ghcr.io/sakana-1314/wx-open:server` | Go 后端（alpine + 静态二进制，非 root 运行） | `8091` |
| `ghcr.io/sakana-1314/wx-open:web` | 前端静态产物 + nginx（反代 `/api`、`/callback` 到 server） | `80` |

```bash
# 1. 建库（一次性）
mysql -u root < db/init.sql

# 2. 准备环境变量
cp deploy/.env.docker.example deploy/.env      # 填 JWT_SECRET / ADMIN_PASSWORD / 微信凭据 / PUBLIC_BASE_URL

# 3. 起服务（数据库用外部 MySQL，不在 compose 内）
cd deploy && docker compose --env-file .env up -d
docker compose --env-file .env ps              # 两个容器都应为 healthy
curl -s http://127.0.0.1:8080/healthz          # {"status":"ok"}
```

- 浏览器访问 `http://<宿主>:8080`（`WEB_PORT` 可改）；公网必须把 443 反代到 web 容器，微信回调地址为
  `https://<域名>/callback/component` 与 `https://<域名>/callback/message/<APPID>`。
- web 容器里的 nginx 已经把 `/callback/` 配成不缓冲、不鉴权、原样转发；外层的 nginx/CDN **不要**再对回调做鉴权或缓冲。
- 不想用容器时，仍可用 `deploy/wx-platform.service` + `deploy/nginx.conf.example` 走二进制部署。
- 单实例约束：作业引擎靠数据库排他锁避免重复下发，**不要对 server 扩容**。

镜像发布流程见 `.github/workflows/build-images.yml`：push 到 `main`（或手动触发）时按变更路径构建两个组件并推送
`:server` / `:web` 两个 tag；PR 只构建不推送。仓库公开时 ghcr 包默认公开，匿名 `docker pull` 可用。

## 五、第三方平台接入指引（务必逐步照做）

### 5.1 前置：注册并创建平台型第三方平台

| 步骤 | 在哪里做 | 由谁做 |
|---|---|---|
| 1 | 注册微信开放平台账号，并完成**开发者资质认证**（官方原文：第三方平台账号审核免费，**开放平台账号认证需缴 300 元**） | 你（服务商） |
| 2 | 创建第三方平台。**必须选「平台型第三方平台账号」**：只有它才有 secret 与代调用能力；「定制化型」只有一个 APPID、无 secret，不能调任何接口 | 你（服务商） |
| 3 | 一个已完成开发者资质认证的开放平台账号，**最多可创建 5 个平台型第三方平台账号** | — |

依据：`docs/wx-docs/extra5_how_to_be_11f.md`。

### 5.2 开发资料字段 ↔ 环境变量对照表

路径：**微信开放平台 → 管理中心 → 第三方平台 → 详情 → 开发配置 → 开发资料**。

| 后台字段 | 填什么 | 对应 `server/.env` 变量 |
|---|---|---|
| 授权事件接收 URL | `{PUBLIC_BASE_URL}/callback/component` | `PUBLIC_BASE_URL`（间接） |
| 消息与事件接收 URL | `{PUBLIC_BASE_URL}/callback/message/$APPID$`（**必须原样带 `$APPID$` 占位**，微信推送时替换为授权方 appid） | `PUBLIC_BASE_URL`（间接） |
| 消息校验 Token | 自定义一串随机字符串，与下面变量保持一致 | `WX_COMPONENT_VERIFY_TOKEN` |
| 消息加解密 Key | **固定 43 个字符**、只含字母与数字 | `WX_COMPONENT_ENCODING_AES_KEY` |
| 消息加解密方式 | **第三方平台只允许「安全模式」**（不可选明文） | — |
| 数据格式 | **第三方平台只允许 XML**（不可选 JSON） | — |
| AppID | 第三方平台详情页可见 | `WX_COMPONENT_APPID` |
| AppSecret | 第三方平台详情页可见，可重置 | `WX_COMPONENT_APPSECRET` |
| — | 上一次的加解密 Key（轮换期间保留，当前 Key 解不开时回退尝试） | `WX_COMPONENT_ENCODING_AES_KEY_PREV` |

后台填写的两个 URL 可以**从概览页直接复制**（`GET /platform/status` 返回 `authorizationEventUrl` 与 `messageEventUrl`），不需要自己拼。

注意事项：

- URL 必须以 `http://` 或 `https://` 开头（分别对应 80 / 443 端口），且**能被微信公网访问**。若 `PUBLIC_BASE_URL` 不是 https，概览页会给出「微信要求回调地址公网可达（https 对应 443 端口）」的告警。
- 回调是**公开路由**（不带 JWT）：安全性来自 `msg_signature` 校验 + 解密后 receiveid 校验。签名**只用 `msg_signature`，绝不用 `signature`**（官方明确警告）；回包只需纯文本 `success`。
- 回调加解密不可用时（未配 Token / Key）服务仍启动，但回调接口返回 400，微信无法完成接入校验。
- 修改开发资料后，**只对「授权测试公众号/小程序列表」内的账号生效，需全网发布后才对全网授权账号生效**（`docs/wx-docs/extra_config.md`）。

### 5.3 IP 白名单

- 所有微信 API 调用都会校验调用者 IP，**只有写进「IP 白名单」的服务器出口 IP 才能合法调用**，否则返回 `61004`（`docs/wx-docs/token_creat_token.md`）。
- 把后端服务器的**公网出口 IP**（不是内网 IP；NAT 出口按实际出口 IP）加进第三方平台的 IP 白名单。多台机器/多出口全部要加。
- 跨 VPC / 动态 IP / 云函数场景要固定出口（弹性公网 IP）。
- 后台该列表**只接受具体 IP，不接受网段**（`10.0.0.0/24` 无效），数量有上限：开放平台后台的常见口径是**最多 100 个**，但**本仓库的微信文档汇总与 `docs/wx-docs/` 未收录该数字，请以后台页面提示为准**。
- 只配置了回调而没配白名单时，回调能收到 `component_verify_ticket`，但用它换 `component_access_token` 会直接失败在 `61004`。

### 5.4 权限集

路径：**微信开放平台 → 第三方平台 → 详情 → 开发配置 → 权限集配置**（`docs/wx-docs/extra8_how_to_service_01c.md`）。

| 权限集 id | 名称 | 是否必需 | 要点 |
|---|---|---|---|
| **18** | 小程序开发与数据分析 | **必需** | 代码上传/页面列表/提审/撤回/发布/回退/灰度/服务状态/基础库/额度查询/体验版二维码/隐私检测全部依赖它。**互斥权限集**：小程序把 18 授权给你之后，**商家无法再在公众平台发版**（避免代码版本互相覆盖） |
| 30 | 小程序基本信息管理 | 可选 | 设置小程序名称/头像/简介/**类目**等基本信息；类目相关能力用它 |

- 互斥规则：**一个账号只能把互斥权限集授予一个服务商**；非互斥权限集一个账号最多同时授权给 15 个服务商（`docs/wx-docs/extra8_how_to_service_01c.md`）。
- 请求授权链接时可用 `categoryIdList`（逗号分隔）显式指定权限集；不传则用**已全网发布**的权限集列表。
- 小程序侧只有勾选了对应权限集才有效：已授权但缺 18 的小程序无法代管代码，概览页会逐条列出这些小程序。

### 5.5 授权发起页域名与授权回调地址

- 把平台**前端域名**填进后台的「授权发起页域名」，并且它必须与授权回调地址（`redirect_uri`，即本站 `{PUBLIC_BASE_URL}/authorize/callback`）的**域名一致**，否则商家扫码后会看到「确认授权入口页所在域名，与授权后回调页所在域名相同」的错误（`server/internal/core/authurl.go` 中的实现注释即针对该报错）。
- 本地调试可把 `PUBLIC_BASE_URL` 指向内网穿透域名，并确保该域名同时写进后台。
- 生成授权链接时 `redirect_uri` 留空则自动使用 `PUBLIC_BASE_URL` 推导出的回调地址。

### 5.6 授权测试号列表（未全网发布时必做）

- 未通过全网发布的第三方平台，**只有「授权测试公众号/小程序列表」内的账号才能完成授权**，这是调试阶段的硬门槛（`docs/wx-docs/extra5_publish_ac2.md`）。
- 把用于联调的测试小程序加进「第三方平台 → 开发资料 → 授权测试公众号/小程序列表」，否则商家扫码会授权失败。

### 5.7 全网发布

- 服务任意小程序（非测试号列表内的账号）前必须完成**全网发布**：路径为「第三方平台详情 → 开发信息 → 开发资料 → 全网发布」。
- 未发布时调用接口会返回 `61028 invalid component`（第三方平台未发布）。
- 发布前会跑**全网发布接入检测**，自检清单：
  1. 授权事件接收 URL 必须能被访问，且对 `component_verify_ticket` 推送**只返回纯文本字符串 `success`**（不要返回 JSON、不要加密、不要带额外字符）——这是官方硬性要求（`docs/reference/wx-open-platform-api.md` 1.2）；
  2. 检测会向「消息与事件接收 URL」推送专用测试消息，需要按官方《全网发布接入检测说明》回包（其中包含客服消息 `TESTCOMPONENT_MSG_TYPE_TEXT` 与 `QUERY_AUTH_CODE:$code$` 两类专用测试消息；**本仓库的微信文档汇总未收录该说明页**，检测前请打开官方页面核对当前要求）；
  3. 两个 URL 使用的 Token / EncodingAESKey 必须与后台填写的一致。
- 本平台对检测推送的处置：先校验 `msg_signature`、解密、校验 receiveid（授权事件 URL 用第三方平台 appid，代收消息用授权方 appid），登记到「日志 → 回调事件」，**再返回 `success`**；检测项失败时按 `docs/runbook.md` 的「收不到 component_verify_ticket」逐条排查。
- 票据长期收不到时，可用官方的 `api_start_push_ticket` 恢复推送（**本平台当前未把它暴露成界面操作**，需运维按 runbook 直接调用，见 `docs/runbook.md`）。

### 5.8 绑定开发小程序（开放平台后台的界面操作，无 API）

- 步骤（`docs/wx-docs/extra_how_to_dev.md`）：
  1. 先在**微信公众平台**注册一个普通小程序，并完善头像、昵称、简介、服务类目；
  2. 再到**微信开放平台 → 第三方平台详情**，把该小程序添加为「开发小程序」。
- 作用：绑定为开发小程序后，**该小程序在微信开发者工具中上传的代码会进入开放平台草稿箱，而不是公众平台**。这正是本平台模板库链路的入口。
- **这是纯界面操作，微信没有提供对应 API，本平台不做也不可能代做**；「最多绑定 30 个」这类数量上限请以后台页面提示为准（本仓库资料未收录该数字）。
- 不要把「绑定开发小程序」与小程序接口权限集 25 的「开放平台账号管理（`bindOpenAccount`，绑定/解绑开放平台账号）」混为一谈：**本平台不做 `bindOpenAccount`**，两者目的不同。

### 5.9 接入检查清单

- [ ] 已创建**平台型**第三方平台，并拿到 AppID / AppSecret。
- [ ] 开发资料四个值（两个 URL、Token、EncodingAESKey）与 `server/.env` 完全一致；加解密方式为安全模式、数据格式 XML。
- [ ] `PUBLIC_BASE_URL` 已改成公网可访问的 https 域名，`curl -i {PUBLIC_BASE_URL}/healthz` 返回 200。
- [ ] 服务器公网出口 IP 已加入 IP 白名单且无遗漏出口。
- [ ] 已配置权限集：18（必需）+ 按需 30。
- [ ] 「授权发起页域名」= 平台前端域名，且与回调地址同域。
- [ ] 调试用测试小程序已加入「授权测试公众号/小程序列表」（未全网发布时）。
- [ ] 已绑定开发小程序（否则开发者工具上传的代码进不了草稿箱）。
- [ ] **验收点：打开平台「概览」页，「票据状态」显示票据正常/新鲜（20 分钟内收到）、「平台令牌」显示令牌有效**，且告警区无未配置/票据缺失提示。
- [ ] 已完成（或明确暂不需要）全网发布。

## 六、日常使用流程

### 6.1 授权一个小程序

1. 进入 **小程序管理**，点右上角**生成授权链接**。
2. 在弹窗里选 `auth_type`：默认 **2（仅小程序）**；只给某个确定的小程序授权时填 `biz_appid`（**优先级高于 `auth_type`**，指定后只有该 appid 的管理员能完成授权）。
3. `redirect_uri` 默认回填平台配置的授权回调地址（`{PUBLIC_BASE_URL}/authorize/callback`），需要时手动覆盖；`category_id_list`（权限集，逗号分隔）留空即用已发布权限集。
4. 提交后得到 `pc_url`、`mobile_url` 与 `qrCodeContent`（同 `pc_url`）。把链接发给小程序管理员，或直接把二维码给对方扫。
5. 管理员确认授权后，微信跳转 `redirect_uri?auth_code=xxx&expires_in=600`，平台前端 `/authorize/callback` 用 `auth_code` 调 `POST /authorizers/authorize` 换取 `authorizer_refresh_token` 并登记；**同时**微信会向授权事件 URL 推送 `authorized` 事件，平台也会自动登记（两条路径都可，重复登记是幂等的）。若回调页因故没完成，等事件推送即可，或点**同步全部**。
6. 预期结果：小程序出现在列表且授权状态为「已授权」，权限集一栏能看到 18。

> **必须勾选权限集 18，否则无法代管代码**。小程序取消授权后，平台标记为「已取消授权」并停止代调用；该类记录才允许删除，已授权记录删除会返回 409。

### 6.2 上传代码到草稿箱 → 模板库

| 步骤 | 在哪里做 | 前提/结果 |
|---|---|---|
| 1 | **微信开发者工具**（或 `miniprogram-ci`），以「开发小程序」身份上传代码 | 该小程序必须**已绑定为开发小程序**（见 5.8）。上传后代码进入开放平台**草稿箱**，**每个开发小程序只保留最新一份** |
| 2 | 平台 **代码模板** 页 → **同步草稿箱** | 对应 `POST /drafts/sync` → `GET /wxa/gettemplatedraftlist`（用 `component_access_token`） |
| 3 | 在草稿行点 **添加到模板库** | 调 `addtotemplate`，成功后可拿到 `template_id`；**模板库中的模板不会被覆盖** |
| 4 | 在模板行点 **设为默认** | 批量作业里不显式选模板时用默认模板；也可写备注便于识别 |

- **模板库上限 200**（官方硬限制）：`GET /templates` 返回 `limit=200`；页面顶部展示用量进度。已满时必须先**删除不再使用的模板**，否则新增报 `85065 template list is full`。
- **标准模板（`template_type=1`）官方已下架**：平台只支持普通模板（`template_type=0`）。误用标准模板传 `ext_json` 会报 `9402203`。
- 微信侧上传（开发者工具 / `miniprogram-ci`）**不由本平台提供**；`directCommit` 也适用于 CI，但它**绕过模板库直接进该小程序的待审核列表**，只适合单目标，不是本平台主链路（主链路是模板库）。

### 6.3 批量上传代码（`commit`）

界面：**批量任务 → 新建批量任务**（四步向导：选类型 → 选小程序 → 配参数并预览 → 确认提交）。

1. **作业类型**选 `commit`（下拉文案「批量上传代码（commit）」）或 `pipeline`（「全流程（pipeline）」）。
2. **目标选择**：全部已授权且启用 / 按分组或标签筛选 / 手动勾选 `appids`。协议语义：`appids` 显式指定优先；否则按 `filter`（`groupName`、`tags`、`onlyDevPermission`、`includeDisabled`）筛选；都不传表示**全部已授权且启用**的小程序（`server/internal/core/selection.go`）。
3. **模板**：选一个 `template_id`（不选则用默认模板）。
4. **版本号模式**：支持内置变量 `{{date}}` `{{time}}` `{{datetime}}` `{{appid}}` `{{nick_name}}` `{{template_id}}` `{{seq}}`（另有 `{{timestamp}}`）。渲染后**必须 ≤64 字符**（官方限制，超出会被本地校验拦住）。
5. **ext_json**：
   - **留空 = 简单模式**：自动生成 `{"extAppid":"<appid>","ext":{...合并后的变量...}}`，小程序侧用 `wx.getExtConfigSync()` 读取 `ext`。适合「同一套代码、不同后端地址/门店号」。
   - **填模板 = 高级模式**：手写 JSON 模板，支持 `{{变量}}` 占位符与 `pages`/`extPages`/`window`/`tabBar`/`subPackages`/`plugins`/`requiredPrivateInfos` 等字段。
   - **ext 变量覆盖**：分「平台默认（运行参数）< 小程序级 `extVars` < 作业级覆盖」三层合并，内置变量优先级最高。
   - 占位符未定义会**本地直接报错**（不会把带 `{{xxx}}` 的请求发出去）；`ext_json` 必须能解析成 JSON 对象，否则报 `85048`/`85013`。
6. **先点「预览最终请求体」**（`POST /jobs/preview`）：dry-run、不调用微信，逐个小程序给出 `endpoint`、最终 `payload`、以及 `problems`（如版本号超长、ext 占位符未定义、缺权限集）。**预览是提交的硬性前置**：存在无效项时无法进入第 4 步；预览后一旦改动表单任一字段，上次预览立即失效，必须重新预览。核对每个小程序的 `ext_json` 无误后再提交。
7. 第 4 步可勾选**试运行（dry run）**：只生成请求体、不真实调用微信。正式执行时提交（`POST /jobs`）→ 在**任务详情**页看逐项进度；失败项能看到 `errcode`、本地中文说明与分类。

要点：

- `ext_json` 在协议层必须是**字符串化的 JSON**（平台已做二次编码）；`ext` 是唯一允许放自定义数据的位置。
- `user_version` ≤64 字符；**上传成功即自动生成体验版**（无需再单独操作），体验版二维码可在小程序详情或 `/releases/{appid}/trial-qrcode` 获取。
- 代码包限制：单个分包/主包 ≤2MB，**服务商代开发小程序总包 ≤20MB**（普通小程序 30MB），超限报 `85044`。
- 同一 `appid` 的上传必须串行（否则 `9402202`），引擎已按 appid 加锁。

### 6.4 批量提审 → 批量发布

1. **先跑「前置体检」**（`POST /preflight`，`purpose` 选 `commit`/`submit_audit`/`release`）：逐项给出授权、权限集 18、小程序资料、类目、隐私指引、域名、额度、模板库余量的 `pass/warn/fail/unknown` 结论与修复指引。**体检不通过的项不要带病提交**。
2. 提审配置（`/audit-profiles`）：维护类目、标题、标签、版本说明、`ugc_declare`、`privacyApiNotUse`、`orderPath`。`item_list` 的 6 个类目字段（`first_class`/`second_class`/`third_class`/`first_id`/`second_id`/`third_id`）**全部来自 `GET /wxa/get_category`（getAllCategoryName）**，类目必须已在小程序侧配置并通过审核，否则 `85008`。
3. 作业类型：
   - 想一站式：选 `pipeline` —— 顺序为 `commit → privacy_check → submit_audit → release`。引擎会**等隐私检测任务结束**才提审（避免 `61039`）、**等审核通过**才发布，等待期间子项状态是 `waiting`。
   - 想分步：先 `commit` 作业，再 `submit_audit` 作业，最后 `release` 作业。
   - **切勿把 `commit` 与 `submit_audit` 塞进同一个重试循环**（官方明确警告），否则每次重试都会重启隐私检测任务，导致 `61039` 反复出现。
4. 额度：提审额度与加急额度都是**服务商级、旗下小程序共用**（`GET /wxa/queryquota` 的 `rest`/`limit`、`speedup_rest`/`speedup_limit`）。额度用尽（`85085`）时引擎**暂停整个作业**而不是逐个小程序失败；补额度后点**恢复**继续跑。加急额度用尽返回 `89405`。
5. 发布：`POST /wxa/release` 是**全量、立即生效**。需要灰度就用 `release` 作业的 `grayPercentage`（0-100 整数，**比例只能递增**，报 `85082`）；灰度计划可查（`GET /releases/{appid}/gray-plan`，`status`：0 初始/1 执行中/2 暂停/3 完毕/4 已删除），可取消灰度（`POST /releases/{appid}/revert-gray`）。
6. 版本回退：`POST /releases/{appid}/revert`（不传 `appVersion` 即回退到上一个版本）。**最多保留最近发布或回退的 5 个版本**；无上一个线上版本、该版本已回退过、或该版本早于回退功能上线时报 `87012`。
7. 审核台账：**审核管理**页可查审核单（`source` 为 `api`/`event`/`poll`），支持撤回（`undo`，每账号每天 ≤5 次、每月 ≤10 次，超限 `87013`）、加急（`speed-up`，需已提审且在待审核队列）与额度查询；`POST /audits/sync` 是事件推送的对账兜底。

## 七、作业与状态说明

### 7.1 作业类型（`JobType`）

| 值 | 说明 | 步骤 |
|---|---|---|
| `commit` | 上传代码（生成体验版） | `commit` |
| `submit_audit` | 提交审核 | `privacy_check` → `submit_audit` |
| `release` | 发布 | `release` |
| `pipeline` | 全流程 | `commit` → `privacy_check` → `submit_audit` → `release` |
| `sync_info` | 同步授权方信息 | 单步 |
| `sync_audit_status` | 对账审核状态 | 单步 |
| `undo_audit` | 撤回审核 | 单步 |
| `speedup_audit` | 加急审核 | 单步 |
| `set_domain` | 配置域名 | 单步 |
| `revert` | 版本回退 | 单步 |
| `toggle_visit` | 服务状态开关 | 单步 |

### 7.2 作业状态（`JobStatus`）

| 值 | 界面文案 | 含义 |
|---|---|---|
| `pending` | 待执行 | 已创建，等待引擎取走或等待启动 |
| `running` | 运行中 | 正在下发 |
| `paused` | 已暂停 | 人工暂停，或额度耗尽/限流被引擎自动暂停；点「恢复」继续 |
| `succeeded` | 已成功 | 全部子项成功 |
| `partial_failed` | 部分失败 | 有失败子项；可「重试失败项」 |
| `failed` | 已失败 | 全量失败 |
| `canceled` | 已取消 | 未开始项被标记取消，已完成的调用不回滚 |
| `interrupted` | 已中断 | 进程重启等原因中断（引擎会把 `running` 复位后断点续跑） |

### 7.3 子项状态（`JobItemStatus`）

| 值 | 界面文案 | 含义 |
|---|---|---|
| `pending` | 待执行 | 排队中 |
| `waiting` | 等待前置 | **等前置条件（隐私检测完成 / 审核通过）或退避重试**，到点自动重新入队 |
| `running` | 执行中 | 正在调用微信 |
| `succeeded` | 成功 | 该步骤成功 |
| `failed` | 失败 | 见 `errcode`/`errorClass`；`permanent` 类不会被「重试失败项」重试 |
| `skipped` | 已跳过 | 因目标不满足条件（如已取消授权、未勾选 18）被跳过 |
| `canceled` | 已取消 | 作业取消时未开始 |

### 7.4 步骤（`JobStep`）

`commit`（上传代码）、`privacy_check`（等隐私检测任务结束）、`submit_audit`（提审）、`release`（发布）。单步作业在实现中使用 `single` 步骤（契约的 `JobStep` 枚举当前只列了前四个，见「写作时发现的不一致」）。

### 7.5 失败分类与引擎动作（`ErrorClass`）

| 分类 | 含义 | 引擎动作 |
|---|---|---|
| `retryable` | 系统繁忙、分钟级限流（`-1`、`89401`、`61039`） | **退避重试**，直到 `JOB_MAX_ATTEMPTS` |
| `token_expired` | 令牌失效/过期（`40001`、`42001`、`61005`、`61006`） | **刷新令牌后重试一次**；`42007` 无法自动恢复，需商家重新授权 |
| `rate_limited` | 频次/额度受限（`85085`、`89405`、`87013`、`9402202`、`45009`） | **暂停整个作业**（额度为服务商级共用），人工补额度后恢复 |
| `environment` | IP 白名单等环境问题（`61004`、`45035`） | **不消耗重试次数**，需人工在开放平台后台修复 |
| `permanent` | 永久失败（`85008`、`85044`、`9402203` 等） | **不重试**，必须改配置/权限/请求体 |
| `wechat_unknown` | 未在本地错误码表登记的码 | 只重试一次，避免把永久失败当可重试 |

分类表在 `server/internal/model/errcodes.go`，**新增错误码分支前先看它**；不要另起一套分类。

## 八、错误码速查表

下表说明与 `server/internal/model/errcodes.go` 的中文说明一致（`本平台处置` 即该表的 Hint + 引擎分类）。

| errcode | 含义 | 分类 | 本平台处置 / 用户该做什么 |
|---|---|---|---|
| `40001` | access_token 无效或不最新 | token_expired | 刷新令牌后重试；核对**令牌类型**（模板库接口必须用 `component_access_token`，用错表现为 `61014`） |
| `42001` | access_token 已过期 | token_expired | 平台自动刷新后重试一次；仍失败查票据新鲜度 |
| `61004` | 调用来源 IP 未注册（第三方平台 IP 白名单） | environment | 把服务器公网出口 IP 加入白名单后重试；**只支持具体 IP，不支持网段**，上限以后台页面为准 |
| `61007` | 该接口权限集未授权给第三方平台 | permanent | 让商家**重新扫码授权并勾选权限集 18**（代码管理必需），平台不重试 |
| `61014` | 必须用 `component_access_token` 调用该接口 | permanent | 用错令牌；模板库 4 个接口与授权方信息接口用 component 令牌，`/wxa/*` 用 authorizer 令牌 |
| `85008` | 当前小程序没有已审核通过的类目 | permanent | 先在小程序侧添加类目并等审核通过；类目字段取自 `getAllCategoryName` |
| `85009` | 已有正在审核的版本 | permanent | 等审核完成，或先撤回审核（注意 `87013` 次数限制） |
| `85013` | 无效的自定义配置（`ext_json` 未正确转义，或其中的路径在模板里不存在） | permanent | 检查 `ext_json` 是否为合法 JSON 对象、`pages`/`extPages` 是否存在于模板；（纯解析失败是 `85048`） |
| `85044` | 代码包超过大小限制 | permanent | 精简代码包：代开发小程序总包 ≤20MB、单包/主包 ≤2MB |
| `85085` | 提审数量已达本月上限（quota 用尽） | rate_limited | 额度**服务商级共用**；作业自动暂停；在「小程序服务商助手」小程序申请临时额度后恢复作业 |
| `85086` | 提审前必须先上传代码 | permanent | 先跑 `commit` 作业（`pipeline` 会自动先 commit） |
| `61039` | 隐私接口检查任务未完成 | retryable | 等约 1 分钟重试；平台 `privacy_check` 步骤会自动等待；**不要把 `commit` 与 `submit_audit` 放同一重试循环** |
| `61040` | `ext.json` 隐私接口未配置或无权限 | permanent | 在 `ext_json` 的 `requiredPrivateInfos` 中声明并申请权限，或声明 `privacy_api_not_use` |
| `87013` | 撤回审核次数已达上限（每天 5 次、每月 10 次） | rate_limited | 次日 0 点恢复每天额度；月度额度下月恢复 |
| `89405` | 本月加急额度已用完 | rate_limited | 与提审额度同源（服务商级共用）；提高提审质量或申请额度 |
| `9402202` | 请勿频繁提交，待上一次操作完成后再提交 | rate_limited | 同一个小程序必须串行；平台已按 appid 加锁，**不要手工并发触发同一小程序的作业** |
| `9402203` | 标准模板 `ext_json` 错误 | permanent | 标准模板仅支持 `{extAppid, ext, window}`；**标准模板官方已下架，改用普通模板** |
| `44002` | empty post data | permanent | 该接口必须传空 JSON `{}`（`release`/`getversioninfo`/`getvisitstatus`/`get_effective_domain` 等）；平台已统一处理，若出现说明空 body 逻辑被改坏 |
| `45009` | 接口调用超过天级别频率限制 | rate_limited | 可用 `clear_quota` 恢复额度（**每账号每月共 10 次清零机会**），或次日重试；平台会暂停作业 |

> 表中 `81039` 是 `61039` 的常见笔误，微信侧不存在 `81039`。未登记的返回码归入 `wechat_unknown` 并只重试一次，日志里能看到原始 `errmsg`。
> 完整码表（共 **134** 条，含 85043~85312、86000~86102、89014~89405、53300 系列等）见 `server/internal/model/errcodes.go`。

## 九、测试与质量

```bash
# 仓库根目录
make check            # 后端 go build/vet/gofmt + 前端 typecheck/build（提交前必跑）
make test             # 后端 go test + 前端 vitest
make e2e-mock         # mock 微信端到端（无需真实凭据）
make gen              # 由 api/openapi.yaml 重新生成两端代码
```

```bash
# 含集成测试（需要库：make db-init 会一并创建 wx_platform_test）
cd server
TEST_DB_DSN='wxplatform:wxplatform_dev_password@tcp(127.0.0.1:3306)/wx_platform_test?charset=utf8mb4&parseTime=True&loc=Local' \
  go test -p 1 ./...
```

- **未设置 `TEST_DB_DSN` 时集成测试自动跳过**，单测与契约/加解密/mock 测试仍会跑。
- 测试**不得依赖真实微信**：一律用 `server/internal/mockwx`，并把 `WX_API_BASE` 指向 `httptest` 服务。
- 前端 `pnpm typecheck`（vue-tsc，strict，零 `any`）与 `go build ./...` 同时通过，即视为契约一致。

### 9.1 CI（GitHub Actions）

| 工作流 | 触发 | 内容 |
|---|---|---|
| `ci-backend.yml` | push / PR 到 `main`（`server/**`、`api/**`） | gofmt → go vet → go build → 生成文件 diff → `go test -p 1`（起真实 MySQL 服务）→ mock 端到端 |
| `ci-frontend.yml` | push / PR 到 `main`（`web/**`、`api/**`） | pnpm install → typecheck → vitest → build → `schema.d.ts` diff |
| `build-images.yml` | push 到 `main` / PR（`server/**`、`web/**`、`api/**`）与手动触发 | 按变更路径构建镜像；**只有 push 到 `main` 与手动触发才推送到 ghcr** |

两个契约 diff 步骤会重新生成 `api.gen.go` / `schema.d.ts` 并比对：**忘记跑 `make gen` 会直接失败**。

## 十、非目标与扩展点

| 项 | 说明 |
|---|---|
| 不代商家处理客服消息 | 代收消息只登记到「日志 → 回调事件」，不做客服会话与自动回复；客服消息必须由商家在小程序侧自行处理 |
| 不做代注册小程序 | 不涉及快速创建/试用小程序转正等注册类接口；小程序需商家自行注册并完成认证后再授权 |
| 只处理小程序 | **不处理公众号与视频号**；代码管理依赖权限集 18 |
| 不支持绕过模板库批量下发 | 批量上传必须走「草稿箱 → 模板库 → 小程序代码」；`directCommit` 只对单个小程序直接提交，不用于批量 |
| 平台密钥不入库 | `WX_COMPONENT_APPSECRET` 只从环境变量读取；只有授权方 `refresh_token` 以密文入库 |
| `bindOpenAccount` 不做 | 与「绑定开发小程序」是两件事：前者是权限集 25 的开放平台账号绑定/解绑接口，后者是开放平台后台的界面操作 |
| 不做 `submit_audit/v2` 与 `auto_audit`/`auto_reject` | **官方文档不存在**这两个接口与自动提审参数（中/英、新/旧路径均未收录）；加速能力走 `speedupaudit` + `queryquota` |
| 不支持标准模板 | 官方已下架标准模板，只支持普通模板 |
| `directCommit` 非主链路 | 它仅单目标、绕过模板库直进待审核列表；本平台主链路是模板库 |
| 单实例部署 | 作业引擎靠数据库排他锁保证不重复下发；**多实例部署需先改造** |
| 多用户 / RBAC 未实现 | 当前只有一个 `admin`（密码来自 `ADMIN_PASSWORD`），无角色与权限模型，操作日志记录 actor 但无法区分到人 |
| 扩展点 | 多实例（分布式锁 + 外部队列）、Redis/MQ 承载作业调度、RBAC、webhook 通知、日志冷归档 |

## 十一、验收清单

- [ ] 未登录访问任何 `/api/v1/*`（除 `POST /auth/login`）返回 401；错误密码登录返回 401 且触发登录限流（`LOGIN_RATE_MAX`/`LOGIN_RATE_WINDOW_SECONDS`）；`GET /auth/me` 返回 `username=admin`。
- [ ] **小程序管理**能生成授权链接（PC/H5/二维码三件套），并完成**一次真实授权**：扫码后管理员看到权限集勾选页（含 18），授权后平台列表出现该小程序且状态为「已授权」。
- [ ] 概览页「票据状态」「平台令牌」正常，告警区无异常提示；`/platform/status` 返回的两个回调 URL 与开放平台后台填写的完全一致。
- [ ] **前置体检**结论正确：故意用缺 18 权限集、无类目、无隐私指引的小程序各跑一次，能分别得到 `fail` 且 `hint` 给出可执行修复建议。
- [ ] **代码模板**页同步草稿箱拿到草稿，添加到模板库拿到 `template_id`，能设为默认模板；模板库满 200 时新增报 `85065` 并在界面展示中文说明。
- [ ] `commit` 作业的**预览与实际下发一致**：预览给出的 `user_version`、`ext_json`（逐小程序不同）与任务详情里留档的请求体一致。
- [ ] `pipeline` 全流程能在 mock 下跑通（`make e2e-mock`），且真实环境下 `privacy_check` 会 `waiting` 到检测结束才提审。
- [ ] 失败项可**重试失败项**恢复；`permanent` 类失败不被重试；额度耗尽时作业**自动暂停**而不是逐个失败。
- [ ] 作业可**暂停 → 恢复**；运行中取消后未开始项标记 `canceled`。
- [ ] **重启后端**后，原 `running` 作业能断点续跑（子项复位为 `pending` 后继续），不会重复下发已成功项。
- [ ] 三类日志可查：微信调用日志能按 `errcode`/`endpoint`/`appid`/`jobId` 过滤；回调事件能看到 `component_verify_ticket`、`authorized`、`unauthorized`、`weapp_audit_*` 的**解密明文**；操作日志能看到作业与授权的管理动作。
- [ ] **请求日志已脱敏**：`api_call_logs.request` 中 `access_token`/`secret`/`refresh_token` 均为 `***`（库里也不应出现 `authorizer_refresh_token` 明文）。
- [ ] 移动端（≤820px）布局可用：侧栏变为汉堡 + 抽屉，工具条控件占满整行，表格可横向滚动，弹窗 ≤94vw，分页为 `simple` 模式。

## 十二、实现状态与如何自行核对

当前状态：**全部能力已实现并通过验证**。可执行下面的命令自行核对：

```bash
cd server && go build ./... && go vet ./... && gofmt -l ./internal ./cmd   # 后端编译与格式
cd web && pnpm typecheck && pnpm test && pnpm build                       # 前端类型/测试/构建
make check        # 一键：后端 build/vet/gofmt + 前端 typecheck/build
make e2e-mock     # 内置模拟微信服务端跑完整链路（无需真实凭据）

# 契约规模与覆盖度核对
grep -c "^  /" api/openapi.yaml                                  # 路径数（当前 53）
grep -c "operationId:" api/openapi.yaml                          # 操作数（当前 66）
grep -oP "operationId: \K\w+" api/openapi.yaml | tr 'A-Z' 'a-z' | sort -u > /tmp/ops.txt
grep -hoP "^func \(\w+ \*Server\) \K\w+" server/internal/handler/*.go | tr 'A-Z' 'a-z' | sort -u > /tmp/impl.txt
comm -23 /tmp/ops.txt /tmp/impl.txt        # 契约有、handler 没有 → 应为空
```

**最近一次核对结果**：

| 核对项 | 结果 |
|---|---|
| 后端 | gofmt 无输出、`go vet ./...` 通过、`go build ./...` 通过 |
| 后端测试 | `go test -p 1 ./...`（带 `TEST_DB_DSN`，真连 MariaDB）11 个包全部 `ok`，含 wxauth 30 例、wxaudit 38 例、wxjob 32 例、wxapi、mockwx、repo、batch、wxcrypt |
| 端到端 | `make e2e-mock` 通过：票据 → 授权 → 模板库 → 批量上传代码 → 批量提审 → 审核结果推送 → 批量发布 全链路断言 |
| 前端 | `pnpm typecheck` 零错误；`vitest` 28 例通过；`pnpm build` 成功 |
| 契约覆盖度 | 53 个路径 / 66 个 operationId，handler 双向差集为空 |
| 前端页面 | 13 个视图均有实现（无占位页） |
| 实机冒烟 | 以 `MOCK_WX=1` 启动二进制后：`/healthz` 200、登录签发 JWT、`/platform/status` 正确给出回调地址与告警、运行参数局部更新生效、回调无签名/伪造签名均 400、`/audit-profiles` 返回种子默认配置 |

> 测试注意：多个集成测试包共用同一个测试库且会清表，**必须加 `-p 1`**（并行跑会互相破坏、随机失败）；
> 端到端测试另有独立库（`wx_platform_e2e`），见 `make e2e-mock`。

其它细节的权威来源：环境变量全集与注释看 `server/.env.example`；运行参数键名看 `server/internal/model/models.go` 的 `Setting*` 常量；返回码与处置看 `server/internal/model/errcodes.go`；微信官方要求看 `docs/reference/wx-open-platform-api.md` 与 `docs/wx-docs/*.md`；开发约定看 `AGENTS.md`。
