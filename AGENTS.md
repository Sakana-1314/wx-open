# AGENTS.md

微信第三方平台管理平台（wx-open）仓库的智能体开发规范。所有改动须遵循本规范。
`README.md` 面向人类（启动步骤、接入指引、验收清单），本文档聚焦「项目怎么分层、如何改代码、如何验证与提交」。

## 1. 项目结构

| 目录 | 说明 |
|---|---|
| `api/openapi.yaml` | **唯一**接口契约（52 路径 / 62 operationId），前后端都从它生成 |
| `server/` | Go + Gin + GORM 后端（`:8091`），作业引擎在进程内运行 |
| `web/` | Vue 3 + TypeScript + Vite + Naive UI 前端（开发服务器 `:5174`） |
| `db/init.sql` | 建库建用户（`wx_platform` / `wx_platform_test` / `wx_platform_e2e`） |
| `deploy/` | 部署样例：`docker-compose.yml`、`.env.docker.example`、systemd 单元、nginx 反代 |
| `docs/` | 接入手册（`onboarding.md`）、运维手册（`runbook.md`）、官方文档抓取件（**只读**） |
| `.github/workflows/` | CI/CD：后端检查、前端检查、镜像构建发布 |

后端分层（新增代码必须落位）：

```
server/internal/
├── config/     环境变量 → 类型化配置（凭据可缺省，缺失时不得 panic，由体检/状态页提示）
├── auth/       JWT 签发校验、鉴权中间件、登录限流
├── database/   GORM 连接、AutoMigrate、种子数据
├── model/      GORM 模型 + 强类型枚举 + 微信返回码处置表（errcodes.go）
├── repo/       数据访问（禁止在 service 里写 SQL）；未找到统一返回 repo.ErrNotFound
├── secretbox/  refresh_token 落库前的 AES-256-GCM 加解密
├── core/       各服务共用的地基：哨兵错误、WeChatError、目标选择、ext_json 渲染、
│               授权链接、运行参数、平台状态（含额度缓存与票据推送运维入口）、
│               Repos 聚合、Env、TokenStore 适配
├── wxcrypt/    微信消息加解密（对齐官方 5 语言示例 + 官方测试向量）
├── wxapi/      微信 HTTP 客户端（分能力、错误分类、QPS 限流、空 body 统一发 {}）
├── wxtoken/    票据与令牌管理（单飞刷新、提前 10 分钟刷新、refresh_token 轮换、全量重拉）
├── callback/   两个公开回调入口（授权事件 URL / 消息与事件 URL），先回 success 再异步处理
├── batch/      作业引擎（步骤门控、按 appid 串行、分类重试、waiting、断点续跑、单实例锁）
├── wxauth/     业务：授权链路、授权方同步、选项、模板库、单小程序域名/页面/服务状态
├── wxaudit/    业务：审核状态与对账、撤回/加急/额度、发布/灰度/回退、前置体检
├── wxjob/      业务：作业创建/预览/控制 + 四类步骤执行器 + 引擎 Store 适配
├── mockwx/     内置模拟微信服务端（无凭据端到端验证）
├── service/    组装：Container（唯一 handler 依赖入口）、Auth/Logs、回调业务分发
├── handler/    实现 oapi-codegen 的 ServerInterface：只做参数绑定与响应写出
└── router/     路由、CORS、统一 JSON 错误、公开回调路由注册
```

**依赖方向自下而上，禁止反向**：`handler → service → wx*(业务) → core → {repo, model}`；
`wxapi/wxcrypt/wxtoken/callback` 不得 import 业务包与 `service`。

## 2. 必读先做

动手前先读：

1. `api/openapi.yaml` — 接口契约。改接口只改它，然后用 `make gen` 重新生成两端。
2. `docs/reference/wx-open-platform-api.md` — 微信官方文档的**本地汇总**（接口全表、字段、错误码、18 项易错点）。
3. `docs/wx-docs/*.md` — 官方文档原始抓取件（单接口字段表以此为准，与汇总冲突时以它为准）。
4. `README.md` — 启动步骤、业务规则、验收清单。
5. 参考资料边界：`docs/wx-docs/` 与 `docs/reference/` 是**只读资料**，不要"顺手整理"或改写；
   `cache/`、`demos/`、`test1.html` 已 gitignore。

## 3. 协作与提交规范

- **仓库**：`https://github.com/Sakana-1314/wx-open`，主分支 `main`（本地与远程统一用 `main`）。
- **作者身份**：所有提交（author 与 committer）统一为 `Sakana <admin@yangrucheng.top>`，SSH 私钥签名
  （`user.signingkey=/root/.ssh/private.key`，`commit.gpgsign=true`）；改写历史后必须重新签名。
- **提交方式**：直接在 `main` 上按功能点提交并推送；跨多文件的大改动可先开 `<type>/<kebab-case-描述>` 分支再合并。
- **提交信息**：中文 + Conventional Commits 前缀（`feat` / `fix` / `docs` / `refactor` / `test` / `chore` / `ci`），
  一行标题 + 空行 + 说明（可选列点）。
- **分功能点提交**：一个逻辑改动一个 commit，不要把无关改动混在一起。
- **工作流命名**：`.github/workflows/` 里的 `name` 用中文（专有名词除外）；job / step 名同样用中文。

## 4. 通用代码规范

- **注释按语言分档**：Go 只写包注释、导出符号注释与「为什么这样做」的关键说明，不写复述代码的注释；
  前端只保留函数级说明（JSDoc 的 `@param`/`@returns` 写全），实现细节与步骤说明一律不写。
- **不做无谓抽象**：分支直接 `if / else` 平铺写完；不为只调用一两次的代码抽 helper 或包装函数。
- **中文注释与错误信息**：与本仓库既有风格一致（`log.Printf("警告：…")`、`fmt.Errorf("…")`）。
- **类型安全**：Go 用类型化结构体与强类型枚举（`Valid()` 校验）；TS 禁止 `any`，用 `unknown` + 窄化。
- **不要硬编码**：微信 API 基址走 `cfg.WxAPIBase`；官方限制值（64 / 200 / 5 次 / 10 次 / 300 / 1000 等）
  集中在 `model` 并在注释里说明来源。

## 5. server（Go + Gin + GORM）

- **入口**：`cmd/server/main.go` 只做「加载 `.env` → 连库 → 迁移/种子 → 令牌 → 回调 → 作业引擎 → 起 HTTP」，
  业务逻辑一律下沉到 `internal/`。构建信息（`buildTime`/`gitSHA`）由 `-ldflags -X main.*` 注入，本地 `go run` 为空。
- **配置**：`internal/config` 把环境变量收敛成类型化 `Config`；`JWT_SECRET` / `ADMIN_PASSWORD` 缺失**拒绝启动**，
  微信凭据缺失**允许启动**（由概览页与前置体检提示「未配置」）。新增可调项同时更新 `server/.env.example`
  与 `deploy/.env.docker.example`，并在 `validateRanges()` 里给出取值范围。
- **数据访问**：只在 `repo` 写 SQL/GORM 查询，service 与 `wx*` 业务包不得直接碰 `*gorm.DB`；
  未找到统一返回 `repo.ErrNotFound`。
- **错误处理**：业务错误用 `core` 的哨兵错误（`ErrNotFound/ErrConflict/ErrValidation/ErrUnauthorized/ErrNotConfigured`）包装；
  微信侧失败统一转成 `core.WeChatError`（保留 `errcode`），由 handler 输出 502 + 具体 errcode，**不要吞掉 errcode**。
- **返回码处置**：新增错误码分支前先看 `model/errcodes.go`；分类语义（retryable / token_expired / rate_limited /
  environment / permanent）直接决定引擎行为，不要另起一套。
- **测试**：`testify` + 表驱动；集成测试在设置 `TEST_DB_DSN` 时才执行，否则跳过；
  **测试不得依赖真实微信**，一律用 `internal/mockwx` 并把 `WX_API_BASE` / `Options.BaseURL` 指向 `httptest` 服务。
- **格式**：`gofmt` / `go vet` / `go build` / `go test -p 1 ./...` 全绿才算完成（见第 8 节）。

## 6. web（Vue 3 + TypeScript）

- **接口类型只从 `src/api/schema.d.ts` 派生**（`openapi-typescript` 生成，**禁止手改**），请求走 `src/api/client.ts`
  的 `openapi-fetch`（Bearer token）。
- **零 `any`**：`vue-tsc` strict；不确定的类型用 `unknown` 再窄化。
- **API base**：`src/api/config.ts` 解析构建期注入的 `__API_HOST__`（Vite `define`，来自 `HOST` 环境变量）；
  未配置时回退相对 `/api/v1`（开发走 Vite proxy、生产走 nginx 反代）。

### 6.1 UI 规范（所有前端改动必须遵守）

沿用同作者的 `隐患闭环系统` / `备件管理系统` 约定（交互与观感保持一致）：

- **按钮必须是按钮**：可点击操作一律 `n-button` 实体按钮；**禁止彩色文字当按钮**、禁止 `text` 型按钮做主要操作。
  行内操作用 `size="small"`；危险操作（删除/回退/取消作业）用 `type="error" secondary` 并二次确认。
- **删除按钮放在编辑/确认弹窗底部左侧**（点击先 `dialog.warning` 再调 DELETE）；后端 409/502 的 message 里已含原因与建议，直接展示。
- **开关一律 `n-switch`，内部不放任何文字**，语义由旁边的表单标签或列标题表达。
- **侧栏白底可折叠**：桌面 220/64px（`show-trigger="bar"`），≤820px 改为顶部汉堡 + `n-drawer`。
- **移动端响应式**：工具条控件窄屏占满整行；表格 `:scroll-x`；弹窗 `max-width: 94vw`；分页窄屏 `simple`。
- **禁止灰色小字说明**：不要用 ≤12px + 灰色做功能说明；改用正式标题、`n-tag`、表单占位符或校验提示；
  次要信息（总数、额度）用正文色。
- **布局惯例**：页面 = `.page` 容器 + `<page-header>` + 卡片化 `n-card`；筛选区放顶部工具条；
  警示/成功色只用于状态标签与真实语义强调。
- 页面通用约定见 `web/src/views/DashboardView.vue`（范例）、`components/PageHeader.vue`、`components/StatusTag.vue`、`utils/format.ts`。

## 7. 微信协议的硬性事实（改代码前必须遵守，来自官方文档）

1. **两套令牌**：模板库 4 个接口与授权方信息接口用 `component_access_token`；所有 `/wxa/*` 代码管理接口用
   `authorizer_access_token`。用错返回 `61014`。权限集 id **18**（小程序开发与数据分析，**互斥**）是代码管理的前提。
2. **回调签名**：只用 `msg_signature`，**绝不用 `signature`**（官方明确警告）。回包只需返回纯文本 `success`。
3. **加解密**：AES-256-CBC，IV = AESKey[:16]，**PKCS#7 块大小 32（不是 16）**，`AESKey = base64decode(EncodingAESKey + "=")`（43 → 32 字节）；
   明文结构 `random(16) + msg_len(4, 大端) + msg + receiveid`。receiveid 要**分流校验**：授权事件 URL 用第三方平台 appid，代收消息用授权方 appid。
   支持 EncodingAESKey 轮换（当前失败回退上一次）。本地错误码 `-40001~-40011` 与微信全局 errcode 是两套体系。
4. **上传代码**：`ext_json` 必须是**字符串化的 JSON**（Go 侧二次编码）；`user_version` ≤64 字符；
   成功即生成体验版；标准模板官方已下架（`9402203`），本平台只支持普通模板。
5. **提审前置**：上传代码后**必须等隐私检测任务结束**（否则 `61039`），且**不要把 commit 与 submit_audit 放在同一个重试循环**；
   类目必须已在小程序侧配置好（`85008`）；`item_list` 的 6 个类目字段全部来自 `GET /wxa/get_category`（getAllCategoryName）。
6. **额度是服务商级、旗下小程序共用**：提审 `GET /wxa/queryquota`（`85085` 用尽）、加急 `speedupaudit`（`89405` 用尽）；
   额度耗尽要**暂停整个作业**，而不是逐个小程序失败。
7. **并发限制**：同账号提交会返回 `9402202`，因此同一 appid 必须串行（引擎已保证），作业步骤之间也要门控。
8. **必须发 `{}` 的接口**：`release`、`getversioninfo`、`getvisitstatus`、`getweappsupportversion`、`get_effective_domain` 等，否则 `44002`。
9. **GET 接口**：`gettemplatelist`、`gettemplatedraftlist`、`get_latest_auditstatus`、`undocodeaudit`、`queryquota`、`revertcoderelease`、`getgrayreleaseplan`、`revertgrayrelease`、`get_page`、`get_qrcode`、`get_code_privacy_info`、`getallcategories`、`getcategory` 等。
10. **运维前置**：所有 API 调用校验来源 IP 白名单（未加白 `61004`，仅支持具体 IP、最多 100 个）；未全网发布时只有「授权测试号列表」内的账号可授权；
    授权托管后小程序只能用第三方平台登记的服务器/业务域名。

## 8. 验证命令（提交前必须全部通过）

后端（`server/` 目录，Go 装在 `/usr/local/go/bin`，Makefile 已注入 PATH）：

```bash
gofmt -l ./internal ./cmd        # 必须为空
go vet ./...
go build ./...
go test -p 1 ./...               # 集成测试在设置 TEST_DB_DSN 时才执行，否则跳过
# 含集成测试（先建库：mysql -u root < db/init.sql）：
TEST_DB_DSN='wxplatform:wxplatform_dev_password@tcp(127.0.0.1:3306)/wx_platform_test?charset=utf8mb4&parseTime=True&loc=Local' go test -p 1 ./...
# ⚠️ 必须加 -p 1：多个集成测试包共用同一个测试库且会清表，并行跑会互相破坏（随机失败）。
#    端到端测试另有专用库，见 make e2e-mock。

# 端到端（内置模拟微信服务端，无需真实凭据）：
TEST_DB_DSN='wxplatform:wxplatform_dev_password@tcp(127.0.0.1:3306)/wx_platform_e2e?charset=utf8mb4&parseTime=True&loc=Local' go test ./internal/e2e/ -v -count=1
```

前端（`web/` 目录）：

```bash
pnpm typecheck   # vue-tsc，strict，零 any
pnpm test        # vitest
pnpm build
```

仓库根目录还有一键检查：`make check`；mock 端到端：`make e2e-mock`。

## 9. 契约同步

改任何接口 / 字段 / 枚举，只改 `api/openapi.yaml`，然后：

```bash
make gen          # 等价于 make gen-go + make gen-web
# 或分别执行：
cd server && oapi-codegen -generate types,gin -package gen -o internal/gen/api.gen.go ../api/openapi.yaml
cd web && pnpm schema
```

`go build ./...` 与 `pnpm typecheck` 同时通过即视为契约一致；生成文件必须随改动一起提交
（CI 会各自重新生成一遍并 `diff`，不一致直接失败）。

## 10. 容器与镜像发布

两个组件各自一个 Dockerfile，构建上下文就是组件目录：

| 组件 | 构建上下文 | Dockerfile | 运行时 | 端口 |
|---|---|---|---|---|
| `server` | `./server` | `server/Dockerfile` | `alpine:3` + 静态二进制（非 root 用户） | `8091` |
| `web` | `./web` | `web/Dockerfile` | `nginx:stable-alpine`（`web/nginx.conf` 经 envsubst 注入后端地址） | `80` |

- **镜像地址固定**：`ghcr.io/sakana-1314/wx-open:server` 与 `ghcr.io/sakana-1314/wx-open:web`。
  **tag 只保留这两个**（外加 `server-sha-<sha>` / `web-sha-<sha>` 供排查），不要引入语义化版本 tag——部署端只认组件名。
- **web 容器反代**：`/api` 与 `/callback` 由容器内 nginx 反代到 `BACKEND_HOST:BACKEND_PORT`（compose 里是 `server:8091`）。
  前端构建期 `HOST` 留空即走同源相对路径；改成绝对地址则前端直连后端，需要后端 CORS 配合。
- **部署样例**：`deploy/docker-compose.yml` + `deploy/.env.docker.example`（数据库用外部 MySQL，不放进 compose，
  避免误删数据卷）。独立部署（非 compose）时用 `deploy/nginx.conf.example` + `deploy/wx-platform.service`。
- **单实例**：作业引擎靠数据库排他锁保证不重复下发，**不要 `scale server`**；多实例改造见 README「非目标与扩展点」。
- 改 Dockerfile 后本地若无法构建（无 docker 环境），至少保证 `go build` / `pnpm build` 通过，并靠 CI 的镜像构建 job 兜底。

## 11. CI/CD（`.github/workflows/`）

| 工作流 | 触发 | 内容 |
|---|---|---|
| `ci-backend.yml` | push / PR 到 `main`（`server/**`、`api/**` 变更） | gofmt → go vet → go build → 契约 diff → `go test -p 1`（真实 MySQL 服务）→ mock 端到端 |
| `ci-frontend.yml` | push / PR 到 `main`（`web/**`、`api/**` 变更） | pnpm install → typecheck → vitest → build → 契约 diff（`schema.d.ts`） |
| `build-images.yml` | push 到 `main` / PR（`server/**`、`web/**`、`api/**` 变更）与手动触发 | 路径过滤出变更组件 → 构建镜像；**仅 `main` push 与手动触发才推送**到 ghcr，PR 只构建 |

- 工作流名、job 名、step 名一律中文；新增工作流照此办理。
- **安全红线**（仓库公开，工作流对公众可见）：严禁硬编码密钥 / 内网地址，一律 `${{ secrets.* }}`；
  镜像推送只用内置 `GITHUB_TOKEN`（`permissions: packages: write`），不引入 PAT。
- 镜像构建走 `dorny/paths-filter` 过滤：只改前端不会重建后端镜像；手动触发固定构建两个组件（用于强制刷新 tag）。
- 免费额度优先：公开仓库的 Actions 免费，但 `cancel-in-progress: true` 与 GHA 层缓存仍要保留，避免重复构建。

## 12. 安全与部署约定

- **密钥不进库、不进日志、不进镜像**：`WX_COMPONENT_APPSECRET` 等只在环境变量（`server/.dockerignore` 已排除 `.env*`）；
  `authorizer_refresh_token` 以 AES-256-GCM 密文入库（`SECRET_ENC_KEY`）；`api_call_logs` 的 request 必须脱敏
  （`access_token`/`secret`/`refresh_token` 替换为 `***`）。
- **回调是公开路由**：不要给它加 JWT，也不要依赖 `Referer`；其安全性来自 `msg_signature` 校验 + 解密后的 receiveid 校验。
  `deploy/` 里的 nginx 必须保持 `/callback/` 不缓冲、不鉴权、不做限流。
- **公网可达是接入前提**：`PUBLIC_BASE_URL` 必须与开放平台后台填写的一致，且必须 https；出口 IP 要在微信 IP 白名单内。
- **数据库**：生产不要把 MySQL 暴露到公网；`db/init.sql` 里的默认密码必须改。
