# 运维处置手册（runbook）

面向**已经在跑**这套平台的运维/值班人员。每个场景统一按四段式写：

> **现象 → 判断 → 处置 → 验证**

约定：

- 所有 `SELECT/UPDATE` 示例使用主库 `wx_platform`，连接方式与 `server/.env` 一致：
  ```bash
  mysql -u wxplatform -p"$(grep '^DB_PASSWORD=' server/.env | cut -d= -f2)" wx_platform
  ```
- 表名与列名取自 `server/internal/model/models.go`（GORM `TableName()` / 蛇形列名）。
- 「微信调用日志」指前端 **日志 → 微信调用日志**（`GET /logs/api-calls`），可按 `appid` / `jobId` / `endpoint` / `errcode` 四个维度过滤；「回调事件」指 **日志 → 回调事件**（`GET /logs/callbacks`，可按 `kind` / `appid` / `infoType` 过滤）。
- **日志中的请求与响应原文已脱敏**：`access_token`、`secret`、`refresh_token` 一律替换为 `***`。排查 token 类问题时**不要指望在日志里看到令牌明文**，只能看 `errcode`、`scope`（`component`/`authorizer`）、`endpoint` 与 `durationMs`。
- 平台内置的定时对账（`server/internal/service/scheduler.go`）：调度循环每 **1 分钟**跑一次节拍，其中审核状态对账每 **5 分钟**、票据新鲜度检查每 **5 分钟**、授权方信息同步每 **60 分钟**、日志清理每 **24 小时**；调度器与作业引擎一样靠数据库排他锁（`wx_platform_scheduler:{DBName}`）保证多实例下只有一个在跑。

---

## 1. 收不到 `component_verify_ticket`

| 阶段 | 内容 |
|---|---|
| **现象** | 概览页「票据状态」显示**票据缺失**；`GET /platform/status` 的 `ticketFresh=false`；后端日志周期性出现 `[scheduler] 告警：component_verify_ticket 不新鲜（距今 N 秒）`；所有需要令牌的接口报「尚未收到 component_verify_ticket」（`wxtoken.ErrNoTicket`）；或先报 `61005`（ticket 过期）/`61006`（ticket 无效） |
| **判断** | 按下面 7 步逐条判定，**每一步都要有明确结论**，不要跳步 |

### 判断（按顺序）

```bash
# ① 平台认为该填进后台的 URL 是什么？
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8091/api/v1/platform/status | python3 -m json.tool | grep -E 'authorizationEventUrl|messageEventUrl|ticketFresh|ticketAgeSeconds|ticketUpdatedAt'
# ② 该 URL 从公网可达吗？（在任意外网机器上跑）
curl -i -X POST https://wx.example.com/callback/component
# ③ 最近一次票据是什么时候到的？
mysql -u wxplatform -p wx_platform -e "SELECT ticket_received_at, last_push_ok_at, updated_at, LENGTH(verify_ticket) AS ticket_len FROM platform_state;"
```

| # | 检查项 | 判定 | 下一步 |
|---|---|---|---|
| 1 | **后台填的授权事件 URL** 与 `authorizationEventUrl` 是否逐字符一致 | 不一致 | 改成后台值与平台值一致（含 `https://`、无尾斜杠、无多余路径） |
| 2 | **URL 公网可达**（DNS 解析、证书有效、反向代理转发、云安全组放行 443） | 连不上/超时/502 | 先修网络与证书；`POST` 必须放行（回调是 POST + XML） |
| 3 | `PUBLIC_BASE_URL` 是否就是对外域名 | 配成了内网地址/`127.0.0.1`/旧域名 | 改 `server/.env` 的 `PUBLIC_BASE_URL` 后重启后端 |
| 4 | **平台是否收到过请求**：看「日志 → 回调事件」筛 `kind=component` | **完全没有记录** → 微信没推到（回第 1/2 步）；**有记录但 `signatureOk=false`** → Token 或 EncodingAESKey 与后台不一致 | 见第 4 条与第 5 条 |
| 5 | **防火墙/反代**是否只放行了 GET | 只放行 GET → 数据表只回限制 405 | 放行 POST；确认反代未丢弃请求体（`client_max_body_size` 足够） |
| 6 | **IP 白名单**是否已配 | 未配 | 回调本身不受影响，但取令牌会 `61004`（见第 4 节） |
| 7 | 平台实例是否**多开了**，两个进程抢 `platform_state` 单行 | 看进程数与启动日志 | 停掉多余实例（作业引擎与调度器都假定单实例锁） |

### 处置

1. **先确认后台的 Token / EncodingAESKey 与 `.env` 逐字符一致**（不一致就无法验签/解密，表现就是「有请求记录但 signatureOk=false」）：
   ```bash
   grep -E '^WX_COMPONENT_(VERIFY_TOKEN|ENCODING_AES_KEY|ENCODING_AES_KEY_PREV)=' server/.env
   awk -F= '/ENCODING_AES_KEY=/{print length($2)}' server/.env   # 必须为 43
   ```
   改完**重启后端**（环境变量只在启动时读取）。
2. **主动恢复推送**（官方手段 `api_start_push_ticket`；`0` 表示「ok（从不正常变正常）或 in a normal state（本来就正常）」）：
   > 【注意】**本平台当前未把该操作暴露成界面或接口**（`api/openapi.yaml` 无对应端点，`server/internal/wxtoken` 内有 `EnsurePushTicket` 能力但无调用入口）。请直接用官方接口调用：
   ```bash
   curl -s -X POST https://api.weixin.qq.com/cgi-bin/component/api_start_push_ticket \
     -H 'Content-Type: application/json' \
     -d "{\"component_appid\":\"$WX_COMPONENT_APPID\",\"component_secret\":\"$WX_COMPONENT_APPSECRET\"}"
   # 期望：{"errcode":0,"errmsg":"ok"} 或 {"errcode":0,"errmsg":"in a normal state"}
   # 40013 invalid appid → 核对 component_appid 拼写与大小写
   ```
   注意：这条命令要在**已加白名单、且出口 IP 属于该白名单**的服务器上执行，否则同样受 `61004` 影响。
3. **用 12 小时内最近可用的票据兜底**：官方票据有效期 **12 小时**，即使推送中断，也还能用最近的票据换 `component_access_token`。平台已把票据持久化：
   ```bash
   # 看最近票据的时间与长度（明文不对外展示，但库里可查）
   mysql -u wxplatform -p wx_platform -e \
     "SELECT ticket_received_at, TIMESTAMPDIFF(MINUTE, ticket_received_at, NOW()) AS age_minutes, LENGTH(verify_ticket) AS len FROM platform_state;"
   ```
   - `age_minutes < 720`（12 小时）→ 令牌仍可换取，**先恢复业务，再修推送**。
   - `age_minutes >= 720` → 票据已过期，必须先修通推送（回到处置第 1、2 步），没有任何本地办法绕过。
4. **恢复后确认令牌能取到**（`platform_state.component_token_expire_at` 被刷新即说明成功）：
   ```bash
   mysql -u wxplatform -p wx_platform -e \
     "SELECT ticket_received_at, component_token_expire_at, (component_token_expire_at > NOW()) AS token_valid FROM platform_state;"
   ```

### 验证

- 概览页「票据状态」在**下一个整 10 分钟推送周期**内变为正常（`ticketFresh=true`，即 20 分钟内收到过）；「平台令牌」显示有效。
- 「日志 → 回调事件」出现新的 `kind=component`、`infoType=component_verify_ticket`、`signatureOk=true`、`processed=true` 的记录。
- 「日志 → 微信调用日志」筛 `endpoint=/cgi-bin/component/api_component_token`，重新触发一次授权/同步后应出现 `ok=true` 记录。
- `[scheduler]` 告警在日志中不再出现。

---

## 2. `component_access_token` 过期 / 取不到

| 阶段 | 内容 |
|---|---|
| **现象** | 模板库、授权方信息等接口报 `40001`（access_token 无效或不最新）、`42001`（已过期）或 `42006`（component_access_token 已过期）；概览页「平台令牌」显示未获取/已过期 |
| **判断** | 看「日志 → 微信调用日志」筛 `endpoint=/cgi-bin/component/api_component_token`：<br>· **没有记录** → 请求根本没发出去（上游 `ErrNoTicket` 或凭据未配置）；<br>· 有记录且 `errcode=61005/61006` → 票据过期/无效 → 走第 1 节；<br>· 有记录且 `errcode=45009` → 天级调用超限；<br>· 有记录且 `errcode=40125` → AppSecret 错（可能刚重置）；<br>· 有记录且 `errcode=61004` → IP 白名单 → 走第 4 节 |

机制背景（官方原文）：令牌有效期 **7200 秒**，官方建议**提前 10 分钟**（1 小时 50 分处）刷新。平台已实现：缓存令牌、**提前 10 分钟**刷新、同一进程内**单飞（singleflight）**合并并发刷新，避免把每日获取次数打爆。票据更新时旧令牌会被立即作废，强制用新票据换取。

### 处置

1. **票据类原因**：走第 1 节（票据是取令牌的前置条件，令牌问题十有八九根因在票据）。
2. **`45009` 天级调用超限**：不要靠重启后端狂刷令牌，先降频（把设置页的 `WX_MAX_QPS` 调小），再考虑清零额度：
   - 平台自身额度用 `component_access_token` 调 `clear_quota`；代调用额度用 `authorizer_access_token`；
   - **每个账号每月共 10 次清零机会**（平台上的清零与 API 清零共享这 10 次），用尽后再调返回 `48006`；
   - 官方另有「使用 AppSecret 重置第三方平台 API 调用次数」的 `POST /cgi-bin/component/clear_quota/v2`。
3. **`40125 invalid appsecret`**：开放平台后台核对 AppSecret；若近期重置过，**旧 AppSecret 立即失效**，必须更新 `WX_COMPONENT_APPSECRET` 并**重启后端**。
4. **凭据缺失**：`GET /platform/status` 返回的 `warnings` 里会明确写「第三方平台凭据未配置…」；补 `server/.env` 的 4 个 `WX_COMPONENT_*` 后重启。
5. **多实例互相踩**：确认只跑一个后端实例（令牌缓存在内存，多实例会各自刷新）。多实例部署属于「非目标」，需先改造。

### 验证

```bash
# 令牌有效期被刷新到未来 2 小时内
mysql -u wxplatform -p wx_platform -e \
  "SELECT component_token_expire_at, TIMESTAMPDIFF(MINUTE, NOW(), component_token_expire_at) AS remain_minutes FROM platform_state;"
```

- 「日志 → 微信调用日志」中 `endpoint=/cgi-bin/component/api_component_token` 最近一条 `ok=true`；
- 重新点一次「代码模板 → 同步草稿箱」能成功（这是对 component 令牌最直接的验活）；
- 日志中不再出现连续 `40001`/`42001`。

---

## 3. `authorizer_refresh_token` 失效

| 阶段 | 内容 |
|---|---|
| **现象** | 某个小程序的接口报 `40001`/`42001` 反复刷新仍失败；或出现 `42007`（用户修改密码导致令牌失效）；或日志/响应里出现 `refresh_token is invalid` |
| **判断** | 在「小程序管理」详情里看该小程序的 `refreshTokenUpdatedAt`；看「日志 → 微信调用日志」筛 `appid=<该小程序>` 与 `endpoint=/cgi-bin/component/api_authorizer_token`：<br>· `errcode=42007` → **必须商家重新授权**，平台无法自愈；<br>· `refresh_token is invalid` → 本地 refresh_token 确实无效（常见是历史存量数据），走全量重拉；<br>· `errcode=61004` → 白名单；<br>· `errcode=61007` → 权限集未授权（第 5 节） |

> 官方说明：「只要商家不解除授权，authorizer_refresh_token **一直有效**」；解除授权后重新授权会**改变** refresh_token。另外，若出现 `refresh_token is invalid`，官方建议的恢复路径就是**重新调 `getAuthorizerList` 获取**。

### 处置

1. **`refresh_token is invalid`（多数情况）→ 用平台「重新拉取令牌」**：界面在 **小程序管理 → 重新拉取令牌**，对应 `POST /authorizers/resync-tokens`，底层即官方 `api_get_authorizer_list` 全量重拉（按 `offset`/`count` 分页，`count` **最大 500**，超出报 `40170`）。拉到的 refresh_token 会**以 AES-256-GCM 密文入库**（密钥 `SECRET_ENC_KEY`）。
2. **`42007` → 只能让商家重新授权**：用户改过微信密码，access_token 与 refresh_token 双失效，**无法自动恢复**。用「生成授权链接」把链接发给商家管理员重扫（此时可指定 `biz_appid` 精确定位到该小程序）。
3. **只影响个别小程序时的最小动作**：先在小程序详情点「同步信息」/「刷新令牌」做单点修复；批量不行再全量重拉。
4. **`SECRET_ENC_KEY` 被改动过**：历史密文将无法解开（表现为解密失败而不是 `refresh_token is invalid`）。此时需要**全量重拉**一次；生产环境请固定 `SECRET_ENC_KEY`（base64 编码的 32 字节），不要让它由 `JWT_SECRET` 派生后再改 `JWT_SECRET`。

### 验证

- 该小程序详情里 `refreshTokenUpdatedAt` 变为刚刚；「刷新令牌」按钮点击后不再报错。
- 「日志 → 微信调用日志」筛 `appid=<该小程序>`，`endpoint=/cgi-bin/component/api_authorizer_token` 最新一条 `ok=true`。
- 跑一次单小程序体检或「同步信息」，能拿到昵称/类目/域名快照。
- 抽查数据库确认 refresh_token **不是明文**：
  ```bash
  mysql -u wxplatform -p wx_platform -e "SELECT appid, LEFT(HEX(refresh_token_cipher),16) AS cipher_prefix, refresh_token_updated_at FROM authorizers WHERE appid='wx1234567890abcdef';"
  ```

---

## 4. `61004` 调用来源 IP 未注册

| 阶段 | 内容 |
|---|---|
| **现象** | 任意微信接口失败，`errcode=61004`、`errmsg=access clientip is not registered`；`errorClass` 为 `environment`；**回调仍能收到 ticket**（这一点极具误导性） |
| **判断** | 服务器**公网出口 IP** 不在第三方平台 IP 白名单。判定命令：`curl -s https://ifconfig.me`（或 `https://api.ipify.org`），与后台白名单逐条比对。常见错因：加了内网 IP、加了跳板机 IP、加了旧机房 IP、NAT 多出口只加了一个 |

### 处置

1. 在服务器上取真实出口 IP（容器内看到的可能是内网 IP，要在宿主机/网关侧确认 NAT 出口）：
   ```bash
   curl -s https://ifconfig.me; echo
   curl -s https://api.ipify.org; echo
   ```
2. 打开 **第三方平台详情 → 开发配置 → IP 白名单**，加入该 IP（**多台机器、多出口、负载均衡出口全部加入**）。
3. 后台该列表**只接受具体 IP、不接受网段**（`10.0.0.0/24` 这类写法无效）；数量有上限，**本仓库微信文档汇总未收录该上限数字，以开放平台后台页面提示为准**。
4. 动态出口（云函数、弹性伸缩、CDN 回源）必须先固定出口（弹性公网 IP / NAT 网关）再谈加白。
5. 不需要重启后端，直接重试调用即可。

### 验证

- 「日志 → 微信调用日志」筛 `errcode=61004`，重新触发一次调用后，新记录 `ok=true`。
- 概览页告警区无相关提示；模板库同步、授权链接生成任一成功即说明白名单生效。
- 注意：`environment` 类错误**不消耗重试次数**，引擎会直接停下等人处理，所以修好白名单后需要**点「恢复」**或重跑作业。

---

## 5. `61007` 权限集未授权 vs `48001` 账号自身无权限

| errcode | errmsg | 含义 | 处置方 |
|---|---|---|---|
| `61007` | `api is unauthorized to component` | **小程序没有把该接口对应的权限集授予本第三方平台**（代码管理需要权限集 **18**） | **商家**重新扫码授权并勾选权限集 18（平台永久失败，不重试） |
| `48001` | `api unauthorized` | **小程序账号自身**没有开通/获得该接口权限（与该第三方平台无关） | **商家**在小程序侧（公众平台）申请或开通该能力；平台无法代改 |
| `61014` | `must use component token for component api` | **用错令牌**：该接口要求 `component_access_token` 却传了 `authorizer_access_token`（模板库 4 个接口与授权方信息接口属于此类） | **平台配置/代码问题**，不是权限问题 |

### 判断

1. 在「小程序管理」详情里看该小程序的权限集列表（`funcInfoIds`）与 `hasDevPermission`；概览页告警区也会逐条列出「以下小程序尚未授权『小程序开发与数据分析』（权限集 18）」。
2. 看失败接口属于哪一类：`/wxa/commit`、`/wxa/submit_audit`、`/wxa/get_page`、`/wxa/release` 等 `/wxa/*` 代码管理接口需要 18，并且必须用 `authorizer_access_token`；`/wxa/gettemplatedraftlist`、`/wxa/addtotemplate`、`/wxa/gettemplatelist`、`/wxa/deletetemplate` 与 `/cgi-bin/component/api_get_authorizer_info` 必须用 `component_access_token`（用错就是 `61014`）。
3. 看「日志 → 微信调用日志」的 `scope` 字段：`component` 表示用的是平台令牌，`authorizer` 表示用的是授权方令牌——这是判断「用错令牌」最快的依据。

### 处置（`61007`）

1. 生成授权链接（`authType=2`，可用 `bizAppid` 指定该小程序）发给商家管理员。
2. 商家扫码后**必须单独勾选「小程序开发与数据分析」**；授权完成后回来点「同步信息」。
3. 提醒商家：权限集 18 是**互斥权限集**，授权给服务商后**小程序无法再通过公众平台发版**（官方原文，见 `docs/reference/wx-open-platform-api.md` 5.0）。这是产品层面的必告知项。
4. 重新跑「前置体检」，`dev_permission` 检查项应变为 `pass`。

### 验证

- 小程序详情 `funcInfoIds` 含 18、`hasDevPermission=true`。
- 重新触发原失败操作（如「代码模板 → 同步草稿箱」或单小程序 `commit`）成功。
- 「日志 → 微信调用日志」中该 `appid` 的最新记录 `ok=true`，不再出现 `61007`。

---

## 6. 提审额度用尽 `85085` / 加急额度用尽 `89405`

| 阶段 | 内容 |
|---|---|
| **现象** | 提审失败 `errcode=85085 submit audit reach limit`；作业状态变为**已暂停**（引擎把 `rate_limited` 类处理为「暂停整个作业」而不是逐个小程序失败）；概览页告警「提审额度已用尽（85085）：批量提审会被暂停，请在『小程序服务商助手』申请临时额度」；加急失败 `89405 本月加急额度已用完` |
| **判断** | 额度是**服务商级、旗下所有小程序共用**（一个池子），不是每个小程序一份。查询：`GET /audits/{appid}/quota` → `GET /wxa/queryquota`，返回 `rest`/`limit`（提审）与 `speedup_rest`/`speedup_limit`（加急）；概览页也会缓存展示（`queriedAt` 为最近查询时间） |

### 处置

1. **不要逐个重试**：额度用尽是池子级问题，重试只会继续失败并把作业搅乱。先在作业详情确认状态是 `paused`。
2. **申请临时额度**：在**「小程序服务商助手」小程序**中提交申请或找人工客服（官方指引 `docs/wx-docs/extra2_submit_quota.md`）。这是微信侧动作，**平台无法代做**。
3. 拿到额度后回到作业详情点 **恢复**（`POST /jobs/{id}/resume`），作业从断点继续，已成功的子项不会重跑。
4. **降低额度消耗**：
   - 提审前**必须先跑「前置体检」**，把 `fail` 项修掉再提审（`85008` 类本地就能拦住的失败不该浪费额度）；
   - 不要对同一个小程序反复「撤回 → 重提」（撤回本身还有 `87013` 次数限制）；
   - `pipeline` 作业比「手工分步 + 反复重试」更省额度，因为它等审核通过才发布，中间不重复提审。
5. **加急额度**（`89405`）与提审额度分开计量；加急后预计 **2-12 小时**出结果，不要为赶时间滥用加急。
6. 若额度显示仍为 0 但你认为已补充：手动点一次额度查询刷新缓存（概览页/`GET /audits/{appid}/quota`），缓存里带 `queriedAt`，避免被旧值误导。

### 验证

- `GET /audits/{appid}/quota` 的 `rest > 0`（或 `speedupRest > 0`）。
- 作业恢复后状态从 `paused` → `running`，失败子项从 `failed` 经「重试失败项」重新入队并变为 `succeeded`。
- 「日志 → 微信调用日志」筛 `errcode=85085`，恢复后不再新增该码的记录。

---

## 7. 隐私检测未结束 `61039` 与隐私接口未配置 `61040`

| 阶段 | 内容 |
|---|---|
| **现象** | 提审报 `61039`「隐私接口检查任务未完成，请稍等一分钟再重试」；或报 `61040`「ext.json 配置的隐私接口无权限 / 代码中含有未配置的隐私接口」 |
| **判断** | ① `61039`：上传代码后微信要跑**隐私检测任务**，任务没结束就提审必报。平台为此在 `submit_audit` 作业前插入了 `privacy_check` 步骤（用 `get_code_privacy_info` 探测，超时上限为运行参数 `privacyCheckMaxWaitSeconds`，默认 600 秒），等待期间子项状态是 **`waiting`（界面文案「等待前置」）**。<br>② `61040`：代码里用了地理位置等隐私接口，但 `ext_json` 没配 `requiredPrivateInfos`，或配了但小程序未获对应 API 权限（相关校验码 `85310`/`85311`/`85312`） |

### 处置

1. **看到 `61039` 先看子项状态**：如果是 `waiting`，**这是正常等待，不要手工干预**；超过 `privacyCheckMaxWaitSeconds` 会转为失败并给出提示，此时再排查。
2. **绝对不要把 `commit` 与 `submit_audit` 放进同一个重试循环**（官方明确警告）：每次重试上传都会**重启检测任务**，结果是 `61039` 永远好不了。本平台的做法是**步骤门控**——同一步骤全部结束后才进入下一步，且 `pipeline` 顺序固定为 `commit → privacy_check → submit_audit → release`。**改动作业流程时不要破坏这个门控**。
3. **手工改坏了门控的迹象**：日志里 `endpoint=/wxa/commit` 与 `/wxa/submit_audit` 交替高频出现，且 `61039` 连续不断。处置：暂停作业 → 只跑一次 `commit` → 等 `privacy_check` 完成 → 再单独跑 `submit_audit`。
4. **`61040`**：二选一——
   - 在 `ext_json` 里声明 `requiredPrivateInfos`（可放在作业的 ext 高级模板或该小程序的 ext 变量里），并确保小程序侧**已申请对应接口权限**；
   - 若确实不使用这些接口，在提审配置里勾选 `privacy_api_not_use`（界面：提审配置的「声明不使用检测出的隐私接口」）。
5. 提审前用**前置体检**（`purpose=submit_audit`）先看 `privacy` 与 `category` 检查项，避免把已知问题送进提审消耗额度。

### 验证

- 任务详情里 `privacy_check` 步骤的子项由 `waiting` → `succeeded`，随后 `submit_audit` 才出现 `running`。
- 「日志 → 微信调用日志」筛 `endpoint=/wxa/security/get_code_privacy_info`，返回的 `without_auth_list` / `without_conf_list` 为空（或已按第 4 条处置）。
- 提审成功后 `errcode` 为 `0`，作业详情的 `wxAuditId` 有值；「审核管理」出现该审核单（`source=api` 或 `event`）。

---

## 8. 并发限制 `9402202`

| 阶段 | 内容 |
|---|---|
| **现象** | 上传/提审报 `9402202`「请勿频繁提交，待上一次操作完成后再提交」；`errorClass=rate_limited`；作业可能被暂停 |
| **判断** | **同一个小程序**的上传/提审必须串行。判定：并发的两个作业是否覆盖了同一个 `appid`？是否有人手工点了两次？是否同一个 `appid` 上同时跑了单应用操作（如 `/releases/{appid}/release`）与批量作业？ |

### 处置

1. 引擎已**按 appid 加锁**（同一小程序在作业内串行），出现该码通常是**跨作业并发**：
   - 检查「批量任务」里是否有多个 `running` 作业选中了重叠的小程序；
   - 暂停其中一个，或等前一个结束再启动；
   - **不要手工并发触发同一小程序的作业**（这是最常见的自伤操作）。
2. 单应用页面（小程序详情里的刷新令牌/体验版二维码等）本身是轻量 GET，不冲突；冲突的是 `commit`/`submit_audit`/`release` 这类写操作。
3. 误报为永久错误时不要重试太密：本码归 `rate_limited`，退避或暂停后重试即可恢复。
4. 长期解决办法：把多个小程序的批量操作合并成**一个**作业（`selection` 选全部或按分组/标签），让引擎统一排队。

### 验证

- 作业详情中同一 `appid` 的子项不再出现 `9402202`，`attempt` 不再增长。
- 「日志 → 微信调用日志」筛 `errcode=9402202`，时间线上不再有重叠调用。
- 作业从 `paused` 恢复后能顺序跑完。

---

## 9. 撤回次数超限 `87013`

| 阶段 | 内容 |
|---|---|
| **现象** | 撤回审核失败，`errcode=87013`「no quota to undo code」；`errorClass=rate_limited` |
| **判断** | 官方限制：**单个账号每天审核撤回次数最多不超过 5 次（每天额度从 0 点开始生效），一个月不超过 10 次**。判定：查「审核管理」里该 `appid` 的撤回记录条数，或看「日志 → 微信调用日志」筛 `endpoint=/wxa/undocodeaudit` 当天的成功条数 |

### 处置

1. 当天额度耗尽 → **次日 0 点自动恢复**；月度额度耗尽 → 下月恢复。不要反复重试。
2. 撤回不是「重提」的前置动作：正确路径是**修好问题后重新上传代码再提审**（`commit` → `privacy_check` → `submit_audit`），而不是撤回再原样提审。
3. 额度用尽时若要停止后续动作，直接**暂停或取消**作业，避免把额度相关的失败项堆在作业里。
4. 撤回次数与提审额度是两套限制，撤回省下来的额度**不会**转移给提审，请分别核算。

### 验证

- 次日 0 点后再点撤回，`errcode` 为 `0`；「审核管理」中该审核单状态变为「已撤回」（`status=3`）。
- 日志中 `87013` 不再新增。

---

## 10. `44002 empty post data`

| 阶段 | 内容 |
|---|---|
| **现象** | `errcode=44002`、`errmsg=empty post data`；`errorClass=permanent` |
| **判断** | 该接口**必须提交一个空 JSON `{}`**，不传 body 就报这个错。涉及 `release`、`getversioninfo`、`getvisitstatus`、`getweappsupportversion`、`get_effective_domain` 等。官方原文：「注意，post 的 data 为空，不等于不需要传 data」。**本平台已在 `wxapi` 层统一为空 body 补 `{}`**，因此正常运行时不该出现这个码 |

### 处置

1. **若真的出现了**：说明**空 body 逻辑被改坏**。检查 `server/internal/wxapi` 中「空 body 统一发 `{}`」的实现与对应单测是否被改动/绕过（有人直接 `http.Post` 或把 body 写成 `nil` 都会触发）。
2. 临时绕过：确认调用方是否绕过了统一的微信客户端封装（例如自写了一段 `http.Client` 调用 `/wxa/release`）。
3. 回滚或修正后必须补一条覆盖该接口的单测（用 `internal/mockwx` + `httptest`，不要打真实微信）。

### 验证

- 「日志 → 微信调用日志」筛 `endpoint=/wxa/release`，最新记录 `ok=true`，且 `request` 中能看到 `{}`。
- 单应用「发布」与批量 `release` 作业都能成功。
- `go test ./internal/wxapi/...` 通过，且新增的单测在去掉 `{}` 逻辑时会失败。

---

## 11. 版本回退失败 `87012`

| 阶段 | 内容 |
|---|---|
| **现象** | 回退报 `errcode=87012`「forbid revert this version release」；`errorClass=permanent` |
| **判断** | 官方给出三种情形：① **没有上一个线上版本**（该小程序还没发布过，或已回退过导致无上一版）；② **此版本为已回退版本**（同一个版本不能回退两次）；③ 该版本是**回退功能上线之前**的版本。另外官方限制：**最多保存最近发布或回退的 5 个版本**，且「当前版本回退后，不能再调用版本回退接口，也不会再查到版本信息」 |

### 处置

1. 先看可回退清单：小程序详情 → 历史版本（`GET /releases/{appid}/history-versions`，最多 5 个）。
2. 清单为空 → 没有可回退目标：只能走**重新上传 + 提审 + 发布**的正向流程（`commit` → `submit_audit` → `release`）。
3. 指定的 `appVersion` 已被回退过 → 换一个版本号，或直接**不传 `appVersion`**，让平台回退到上一个版本（`POST /releases/{appid}/revert` 的默认行为）。
4. 回退成功后**不要再对该版本做任何回退操作**（接口不再返回该版本信息，属预期行为）；「发布管理」台账里能看到 `action=revert` 的记录。
5. 回退属于线上影响操作，按钮为危险操作需二次确认；操作人会记入「日志 → 操作日志」。

### 验证

- `GET /releases/{appid}/version`（`getversioninfo`）的 `releaseVersion` 已变回目标版本，且 `releaseTime` 刷新。
- 「发布管理」出现一条 `action=revert` 记录。
- 再次查询历史版本，已回退的版本不再出现在列表中。

---

## 12. 作业卡住 / 进程重启后的续跑

| 阶段 | 内容 |
|---|---|
| **现象** | 作业长时间停在 `running` 但子项不再变化；或后端重启后作业状态异常；启动日志出现「警告：作业引擎未启动（…）：平台仍可浏览与创建作业，但不会执行」 |
| **判断** | ① **引擎没拿到单实例锁**（日志含 `另一个实例正在运行作业引擎（数据库排他锁被占用）`）→ 有别的实例在跑，或上一实例的锁没释放；<br>② 子项全是 `waiting` → **正常等待**（等隐私检测结束 / 等审核通过 / 退避重试），看 `nextRunAt`；<br>③ 子项为 `running` 但 `startedAt` 很久没更新 → 进程崩溃或重启导致残留；<br>④ 作业被自动暂停（额度/限流）→ 见第 6 节 |

```bash
# 当前作业与子项分布
mysql -u wxplatform -p wx_platform -e "
SELECT status, COUNT(*) FROM batch_jobs GROUP BY status;
SELECT job_id, step, status, COUNT(*) c, MIN(started_at) oldest, MAX(finished_at) newest
FROM batch_job_items GROUP BY job_id, step, status ORDER BY job_id DESC LIMIT 30;"

# 是否有残留 running 子项（正常重启后引擎会立即复位为 pending）
mysql -u wxplatform -p wx_platform -e \
  "SELECT COUNT(*) AS stuck_running FROM batch_job_items WHERE status='running' AND started_at < NOW() - INTERVAL 10 MINUTE;"

# 排他锁持有情况（MySQL 5.7+/MariaDB 10.5+）
mysql -u wxplatform -p wx_platform -e "SELECT * FROM performance_schema.metadata_locks WHERE OBJECT_NAME LIKE '%wx_platform%';"
```

### 处置

1. **引擎未启动**（拿不到锁）：
   - 确认只有一个后端进程：`ps -ef | grep 'cmd/server' | grep -v grep`，停掉多余实例；
   - 确认真实是「另一个活着的实例」而不是死锁残留（数据库连接断开后锁会自动释放，重启数据库或等待即可）；
   - 锁名形如 `wx_platform_job_engine:<DB_NAME>`，调度器锁名形如 `wx_platform_scheduler:<DB_NAME>`；**同一库上有两个不同 `DB_NAME` 的实例互不干扰但也互不共享**，别把不同库的实例当成同一个。
2. **重启后的断点续跑是设计行为**：引擎启动时会把残留的 `running` **复位为 `pending`**，然后继续执行未完成作业；已 `succeeded` 的子项不会重跑。因此**重启后端是安全的**，不需要手工改库。
3. **确实卡住且引擎在跑**：先看该作业有哪些子项在执行、`nextRunAt` 是否在未来（`waiting` 属正常）。若长期无变化：
   - 点**暂停** → 观察子项是否停止推进 → 再点**恢复**（这一步会重新入队 `waiting`/`pending` 子项，是最温和的解法）；
   - 仍不行再**取消**作业（未开始项标记 `canceled`，已完成的微信侧结果**不会回滚**），修好原因后新建作业重跑失败的那部分（用 `appids` 精准指定）。
4. **不要手工 UPDATE `batch_job_items.status`**：会让引擎的内存视图与库不一致；唯一被设计允许的批量变更就是引擎启动时的 `running → pending` 复位。
5. **卡住同时进程在疯狂重试**：把设置页的 `JOB_MAX_ATTEMPTS`（1-8）与 `WX_MAX_QPS`（1-50）调小，先止血再定位根因（通常仍是 `61004`/`48001`/额度类）。

### 验证

- 引擎启动日志出现「批量作业引擎已启动（并发上限由设置页控制）」，且无锁告警。
- 残留 `running` 子项查询结果为 0；原作业状态从 `interrupted`/`running` 推进到 `succeeded`/`partial_failed`。
- 作业详情的进度条继续变化，`finishedAt` 更新。
- 「日志 → 操作日志」能看到暂停/恢复/取消的 actor 与时间。

---

## 13. 数据库与日志清理

| 阶段 | 内容 |
|---|---|
| **现象** | 磁盘持续增长；`api_call_logs` / `callback_events` 表越来越大；查询日志页变慢 |
| **判断** | 平台已内置清理任务（`scheduler.pruneLogs`，**每 24 小时**一次），按设置页的**「日志保留天数」**（`runtime_settings.log_retention_days`，环境变量默认 `LOG_RETENTION_DAYS=30`，范围 1-365）删除早于该天数的 `api_call_logs` 与 `callback_events` 记录。判断：看后端日志有没有 `[scheduler] 已清理 N 条微信调用日志（保留 X 天）`；若从未出现，检查 `log_retention_days` 是否 ≤0、以及调度器是否因未拿到锁而未运行 |

### 处置

1. **调整保留天数**：界面 **设置 → 运行参数 → 日志保留天数**（也可 `PUT /platform/settings`，字段 `logRetentionDays`，范围 1-365）。排查故障期间可临时调到 30-90 天，稳定后调回。
2. **手工清理**（大表一次性瘦身时）：
   ```bash
   # 先估算待清理量，避免长事务
   mysql -u wxplatform -p wx_platform -e "
   SELECT COUNT(*) FROM api_call_logs  WHERE created_at < NOW() - INTERVAL 30 DAY;
   SELECT COUNT(*) FROM callback_events WHERE received_at < NOW() - INTERVAL 30 DAY;"

   # 分批删除（每批 5000 条，避免锁表与 undo 膨胀），重复执行直到影响行数为 0
   mysql -u wxplatform -p wx_platform -e "
   DELETE FROM api_call_logs WHERE created_at < NOW() - INTERVAL 30 DAY LIMIT 5000;"

   # 回收空间（InnoDB；大表建议低峰期执行，或重建表）
   mysql -u wxplatform -p wx_platform -e "OPTIMIZE TABLE api_call_logs, callback_events;"
   ```
   > 「日志 → 微信调用日志」的 `created_at` 有单列索引，按时间删除走索引。
3. **不要清理这些表**：`platform_state`（票据/令牌状态，删了会丢最近票据与令牌缓存）、`token_cache`（授权方令牌缓存）、`authorizers`/`audit_records`/`release_records`（台账）、`batch_jobs`/`batch_job_items`（作业与断点续跑依据）、`runtime_settings`（运行参数）。
4. **回调事件的幂等依赖 `callback_events.dedupe_key`**：清空该表会让微信重推的历史事件重新入库（一般无害），但**不要**为了「看起来干净」在业务高峰期清表。
5. **数据库备份**：清理前先备份；恢复票据/令牌状态比恢复代码重要得多。

### 验证

- 后端日志出现 `[scheduler] 已清理 N 条微信调用日志（保留 X 天）`（N>0）。
- 清理后表行数与磁盘占用下降：
  ```bash
  mysql -u wxplatform -p wx_platform -e "
  SELECT table_name, table_rows, ROUND(data_length/1024/1024,1) AS data_mb, ROUND(index_length/1024/1024,1) AS idx_mb
  FROM information_schema.tables WHERE table_schema='wx_platform' ORDER BY data_length DESC LIMIT 10;"
  ```
- 「日志 → 微信调用日志」仍能正常分页与按 `errcode` 过滤（说明没误删索引或整表）。

---

## 14. 用日志定位问题（通用手法）

| 阶段 | 内容 |
|---|---|
| **现象** | 任何「某个操作失败了，但不知道错在哪一步」 |
| **判断** | 三类日志各有分工：<br>· **微信调用日志**（`/logs/api-calls`）→ 看**我们发出的请求**与微信的响应；<br>· **回调事件**（`/logs/callbacks`）→ 看**微信推给我们的东西**；<br>· **操作日志**（`/logs/operations`）→ 看**人做了什么** |

### 处置（推荐排查顺序）

1. **先锁定单次失败**：在「批量任务 → 任务详情」里找到失败子项，记下 `appid`、`errcode`、`errorClass`、`step` 与 `jobId`。
2. **再看微信调用日志**，按下面优先级过滤：

   | 过滤条件 | 什么时候用 |
   |---|---|
   | `jobId=<作业 id>` | 把某一次批量作业的所有调用按时间排开，看失败点与前后调用 |
   | `errcode=<码>` | 判断是**系统性**（多条不同 appid 同码 → 环境/额度/权限类）还是**单点**（只有一个小程序） |
   | `endpoint=<路径>` | 只关心某一类调用（如 `/wxa/submit_audit`、`/cgi-bin/component/api_component_token`） |
   | `appid=<小程序>` | 单小程序专项排障 |

   每条记录可看到：`scope`（`component`/`authorizer`，判断**是否用错令牌**）、`method`、`httpStatus`、`ok`、`errcode`、`errmsg`、`errorClass`、`durationMs`、`createdAt`，以及 `request`/`response` 留档。
3. **判断是「我们没发出去」还是「发了被拒」**：日志里**没有**该 `endpoint` 的记录 → 请求根本没发（本地校验拦住、上游前置步骤失败、引擎未运行）；有记录但 `errcode!=0` → 微信侧拒绝，按第八节/本手册对应场景处置。
4. **回调侧问题看回调事件**：筛 `kind=component`（授权事件 URL）或 `kind=message`（消息与事件 URL）；看 `infoType`（`component_verify_ticket` / `authorized` / `unauthorized` / `updateauthorized` / `weapp_audit_*`）、`signatureOk`、`processed`、`processNote` 与 `decrypted`（解密明文）。`signatureOk=false` 一定是 Token/AESKey 与后台不一致。
5. **谁动了什么看操作日志**：筛 `action`，字段含 `actor`、`targetType`、`targetId`、`detail`、`ip`、`createdAt`。用于回答「这个作业是谁建的/谁取消的」。

### 验证与红线

- **请求/响应原文已脱敏**：`access_token`、`secret`、`refresh_token` 均为 `***`。如果你在日志里**看到了令牌明文**，这是**安全缺陷**，必须立刻按 `AGENTS.md` 的「密钥不进库、不进日志」修 `wxapi` 的记录器，并轮换已泄漏的凭据。
- `authorizer_refresh_token` **以 AES-256-GCM 密文入库**（`SECRET_ENC_KEY`）；库里出现明文同样按缺陷处理。
- 排查完成后，把临时调大的 `logRetentionDays`、调小的 `wxMaxQps` 等运行参数**改回默认**，避免长期偏离。

---

## 附录：一张速查表

| errcode | 分类 | 一句话处置 | 本手册节号 |
|---|---|---|---|
| `61004` / `45035` | environment | 出口 IP 加白名单（仅具体 IP） | 4 |
| `61005` / `61006` | token_expired | 换最近可用票据；必要时 `api_start_push_ticket` | 1 |
| `40001` / `42001` / `42006` | token_expired | 刷新后重试一次；查票据与 `45009` | 2 |
| `42007` | token_expired | 商家改过密码，**必须重新授权** | 3 |
| `refresh_token is invalid` | — | 「重新拉取令牌」= `api_get_authorizer_list` 全量重拉 | 3 |
| `61007` | permanent | 商家重扫并勾选权限集 18 | 5 |
| `48001` | permanent | 小程序自身没开通该能力，商家在小程序侧处理 | 5 |
| `61014` | permanent | 用错令牌（模板库类接口要 component 令牌） | 5 |
| `85085` | rate_limited | 服务商级额度共用；「小程序服务商助手」申请；作业已自动暂停 | 6 |
| `89405` | rate_limited | 加急额度用尽 | 6 |
| `61039` | retryable | 等隐私检测；**别把 commit 与提审放同一重试循环** | 7 |
| `61040` | permanent | `requiredPrivateInfos` 声明或 `privacy_api_not_use` | 7 |
| `9402202` | rate_limited | 同一 appid 串行，别手工并发触发 | 8 |
| `87013` | rate_limited | 撤回每天 5 次/每月 10 次，次日 0 点恢复 | 9 |
| `44002` | permanent | 必须发 `{}`；平台已处理，出现说明逻辑被改坏 | 10 |
| `87012` | permanent | 无上一版本/已回退过/最多保留 5 个版本 | 11 |
| `45009` | rate_limited | `clear_quota`（**每账号每月 10 次**）或次日重试 | 2 |
| `85065` | permanent | 模板库上限 200，先删模板 | — |
| `85044` | permanent | 代开发总包 ≤20MB、单包 ≤2MB | — |
| `85008` | permanent | 小程序侧先配类目并等审核通过 | — |

分类语义（`retryable` / `token_expired` / `rate_limited` / `environment` / `permanent` / `wechat_unknown`）与引擎动作的对应关系见 `README.md` 第 7.5 节；完整返回码表见 `server/internal/model/errcodes.go`。
