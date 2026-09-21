# 微信开放平台「第三方平台 / Third-party Platform」后端实现技术参考

> **文档来源与采集说明**
> 本报告基于 **2026-09-21** 抓取的微信官方文档（`developers.weixin.qq.com/doc/oplatform/...`）逐页解析（HTML→文本）后编写，
> 所有字段均来自官方页面表格；凡官方页面中**没有**出现的内容，均显式标注「文档未明确 / 未收录 / 未能访问」，不做推测填充。
>
> 抓取时官方文档已被重构为 `https://developers.weixin.qq.com/doc/oplatform/openApi/OpenApiDoc/...` 体系（旧路径
> `Third-party_Platforms/2.0/api/...` 部分仍可访问，部分已 404）。两者内容略有差异，本报告**并列标注**。
>
> 相关采集脚本与原始页面缓存位于同目录：`fetch.py`、`cache/`、`docs/`、`demos/`。
>
> 单元约定：表中"token"一列 —— `component_access_token` 表示第三方平台自身令牌；`authorizer_access_token` 表示代授权方调用令牌。

---

## 目录

1. [令牌体系（token model）](#1-令牌体系token-model)
2. [预授权与授权](#2-预授权与授权)
3. [授权方信息](#3-授权方信息)
4. [小程序代码模板库（第三方平台侧）](#4-小程序代码模板库第三方平台侧)
5. [为授权小程序上传代码 / 提审 / 发布](#5-为授权小程序上传代码--提审--发布)
6. [加解密规范（WXBizMsgCrypt）](#6-加解密规范wxbizmsgcrypt)
7. [错误处理与频率限制](#7-错误处理与频率限制)
8. [官方 Demo 代码](#8-官方-demo-代码)
9. [附录 A：未能确认 / 文档未明确项](#附录-a未能确认--文档未明确项)
10. [附录 B：引用链接](#附录-b引用链接)

---

## 0. 全局前置条件（务必先读）

| 项 | 内容 | 来源 |
|---|---|---|
| 出口 IP 白名单 | **所有 API 调用都会校验调用者 IP**，只有第三方平台「白名单 IP 列表」内的 IP 才能合法调用，其他一律拒绝（错误码 `61004 access clientip is not registered`） | [Token生成介绍](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/creat_token.html)、[接口调用指南](https://developers.weixin.qq.com/doc/oplatform/openApi/ticket-token/api_getcomponentaccesstoken.html) |
| 服务器出口 | 接口必须在服务器端调用，不可在小程序/网页/APP 前端直接调用 | 各 API 文档页首提示 |
| 数据格式 | 第三方平台的消息推送**只允许 XML**、**只允许安全模式**（不可选明文） | [消息推送](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/message_push.html) |
| 代调用的本质 | 代商家调用小程序 API 时，**把 `access_token` 换成 `authorizer_access_token`** 即可，其余 URL/参数不变；能否调用成功取决于商家是否把对应权限集授予了该第三方平台（失败常见 `48001`、`61007`） | [如何代商家调用接口](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/getting_started/how_to_call_api.html) |

---

## 1. 令牌体系（token model）

### 1.1 component_access_token（第三方平台令牌）

| 项 | 值 |
|---|---|
| URL | `POST https://api.weixin.qq.com/cgi-bin/component/api_component_token` |
| Method | `POST`（JSON body） |
| 鉴权 | **无需任何 token**（用 `component_appsecret` + `component_verify_ticket` 换 token） |
| 接口英文名 | `getComponentAccessToken` |
| 云调用 | 不支持 |
| 适用范围 | 仅「第三方平台」账号类型 |

**请求体（Request Payload）**

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `component_appid` | string | 是 | 第三方平台 appid |
| `component_appsecret` | string | 是 | 第三方平台 appsecret |
| `component_verify_ticket` | string | 是 | 微信后台推送的 ticket |

**返回体（Response Payload）**

| 参数名 | 类型 | 说明 |
|---|---|---|
| `component_access_token` | string | 第三方平台 access_token |
| `expires_in` | number | 有效期，单位：秒（实测/示例为 `7200`） |

```json
// 请求
{ "component_appid": "appid_value", "component_appsecret": "appsecret_value", "component_verify_ticket": "ticket_value" }
// 返回
{ "component_access_token": "61W3mEpU...", "expires_in": 7200 }
```

**有效期 / 刷新时机（官方原文）**

> 「令牌（component_access_token）是第三方平台接口的调用凭据。令牌的获取是有限制的，每个令牌的有效期为 2 小时，请自行做好令牌的管理，**在令牌快过期时（比如 1 小时 50 分），重新调用接口获取**。」

即：有效期 **7200s**，官方建议**提前 10 分钟**（1h50m 处）刷新 —— 这正是任务中提到的 "10-minute-before-expiry rule"。
实现建议：`缓存 token + expires_at`，在 `expires_at - 600s` 触发异步刷新。

**失败时的错误码（该接口文档原文）**

| 错误码 | 英文描述 | 中文描述 / 处理 |
|---|---|---|
| `-1` | system error | 系统繁忙，稍后重试 |
| `40001` | invalid credential, access_token is invalid or not latest | AppSecret 错误或 token 无效 |
| `40013` | invalid appid | 不合法的 AppID |
| `40125` | invalid appsecret | 无效的 appsecret |
| `41004` | appsecret missing | 缺少 secret 参数 |
| `45009` | reach max api daily quota limit | 调用超过天级别频率限制，可调用 `clear_quota` 恢复额度 |
| `47001` | data format error | 解析 JSON/XML 内容错误；post 数据中参数缺失 |
| `48001` | api unauthorized | 功能未授权 |
| `61004` | access clientip is not registered | **第三方平台出口 IP 未设置**（加入白名单后重试） |
| `61005` | component ticket is expired | ticket 过期 |
| `61006` | component ticket is invalid | ticket 无效 |
| `61011` | invalid component | 无效的第三方平台 |

> ⚠️ 关于「每日获取次数上限」：官方 `api_component_token` 页面**只写了"令牌的获取是有限制的"，没有给出具体数字**；超出时返回 `45009`。数字需以微信公众平台/开放平台后台页面显示为准 → 标注为**文档未明确**。

---

### 1.2 component_verify_ticket 的获取（授权事件接收 URL 推送）

| 项 | 内容 |
|---|---|
| 推送方式 | 第三方平台**创建审核通过后**，微信服务器向其「**授权事件接收 URL**」**每隔 10 分钟**以 `POST` 方式推送一次 `component_verify_ticket` |
| 有效期 | **12 小时**（比 component_access_token 更长） |
| 必须响应 | **接收 POST 请求后，只需直接返回字符串 `success`**（明文，不加密） |
| 建议 | 保存最近可用的 ticket；即使某次推送失败，也可用上一次可用 ticket 继续换取 component_access_token |
| 推送内容 | **加密**（安全模式），解密后 XML： |

```xml
<xml>
  <AppId>some_appid</AppId>                       <!-- 第三方平台 appid -->
  <CreateTime>1413192605</CreateTime>             <!-- 时间戳(秒) -->
  <InfoType>component_verify_ticket</InfoType>    <!-- 固定值 -->
  <ComponentVerifyTicket>some_verify_ticket</ComponentVerifyTicket>
</xml>
```

> 另有接口 `POST https://api.weixin.qq.com/cgi-bin/component/api_start_push_ticket`（接口英文名 `startPushTicket`），
> 请求体 `{ "component_appid": "...", "component_secret": "..." }`，返回 `{errcode, errmsg}`，
> 用于「启动 ticket 推送服务」（错误码：`0` ok/in a normal state、`40013` invalid appid）。
> 该接口**无需 token**，也没有单独文档中的频率限制说明。

来源：[验证票据](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/component_verify_ticket.html)、[消息推送](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/message_push.html)、[启动票据推送服务](https://developers.weixin.qq.com/doc/oplatform/openApi/ticket-token/api_startpushticket.html)

---

### 1.3 平台自身 token 与 授权方 token 的区别

官方「[authorizer_access_token 生成说明](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/creat_token.html)」给出的完整 7 步链路：

| 步骤 | 说明 |
|---|---|
| 1 | 配置**授权事件 URL**，接收 `component_verify_ticket`（每 10 分钟推送，有效期 12h） |
| 2 | 用 ticket 调 `api_component_token` 换 `component_access_token`（有效期 2h） |
| 3 | 用 `component_access_token` 调 `api_create_preauthcode` 拿 `pre_auth_code` |
| 4 | 用 `pre_auth_code` 构建授权链接，引导商家授权，回调得到 `authorization_code`（也有过期时间，回调与推送中都会给出） |
| 5 | 用 `authorization_code` + `component_access_token` 调 `api_query_auth`（getAuthorizerRefreshToken）换 `authorizer_refresh_token` + 授权信息 |
| 6 | 用 `authorizer_refresh_token` 调 `api_authorizer_token`（getAuthorizerAccessToken）换 `authorizer_access_token` |
| 7 | 用 `authorizer_access_token` 代商家调用 API |

| 维度 | component_access_token（平台自身） | authorizer_access_token（授权方） |
|---|---|---|
| 归属 | 第三方平台自己 | 某个已授权的小程序/公众号 |
| 获取 | `api_component_token`（无 token 鉴权） | `api_query_auth`（首次）/ `api_authorizer_token`（刷新） |
| 有效期 | 7200s | 7200s |
| 用在哪 | **所有 `/cgi-bin/component/*` 平台管理接口**、`/wxa/gettemplatedraftlist`、`/wxa/addtotemplate`、`/wxa/gettemplatelist`、`/wxa/deletetemplate` 等 | **代商家调用**的接口：`/wxa/commit`、`/wxa/get_page`、`/wxa/submit_audit`、`/wxa/release`、`/wxa/get_qrcode` … |
| URL 参数名 | `access_token=` 或 `component_access_token=`（见各接口） | `access_token=` |
| 传错时 | — | `61014 must use component token for component api`（应用 component_access_token 却用了别的） |

**authorizer_refresh_token 有效期（官方 FAQ 原文）**

> 「只要商家不解除授权，**一直有效**（解除授权 → 重新授权给第三方平台，会改变 authorizer_refresh_token）。
> 可以调用接口 `getAuthorizerList` 获取所有已授权账号的 authorizer_refresh_token。
> 注意：authorizer_refresh_token 返回空的情况，需要调用 `getAuthorizerRefreshToken` 接口来换取 authorizer_refresh_token。
> 若授权码已过期，可以触发更新授权来获取新的 authorizer_refresh_token。」

> 「出现 `refresh_token is invalid` 怎么办？」——先自查调用 `getAuthorizerAccessToken` 时 `authorizer_refresh_token` 是否真的传了值（常见是传了 `null`）；确认传了值还报错，说明该 refresh_token 确实无效，可重新调 `getAuthorizerList` 获取。

---

### 1.4 component_appsecret 重置

官方「[如何查看和重置 AppSecret](https://developers.weixin.qq.com/doc/oplatform/developers/dev/appid.html)」：

- 第三方平台的 AppID/AppSecret 在**微信开放平台**获取：邮箱密码登录 → 管理中心 → 第三方平台详情页 → 查看 AppID/AppSecret，并可在此**重置 AppSecret**。
- 原文注意事项：
  1. 「平台不储存和显示 AppSecret，如已忘记 AppSecret，则可通过「重置」功能重新生成开发者密钥（AppSecret），并需妥善保存。」
  2. 「如果重置 AppSecret，**旧的 AppSecret 会失效**，因此需要慎重……需尽快更新 AppSecret 信息。」
- 重置后：`api_component_token` 用旧 secret 会返回 `40125 invalid appsecret`；须更新配置后重新获取 token。
- 补充：第三方平台开发配置页面「所有配置均可修改」；开发资料修改后**只对「授权测试公众号/小程序列表」内的账号生效**，**需全网发布后**才对全网授权账号生效。([第三方平台开发配置](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/operation/thirdparty/config.html))

> 「重置 appsecret 后是否有冷却期 / 是否立即生效于已签发 token」：文档**未明确**。

---

### 1.5 令牌相关接口一览

| 接口 | Method + URL | token | Body | 返回 |
|---|---|---|---|---|
| 获取令牌 | `POST https://api.weixin.qq.com/cgi-bin/component/api_component_token` | 无 | `component_appid`, `component_appsecret`, `component_verify_ticket` | `component_access_token`, `expires_in` |
| 启动票据推送服务 | `POST https://api.weixin.qq.com/cgi-bin/component/api_start_push_ticket` | 无 | `component_appid`, `component_secret` | `errcode`, `errmsg` |
| 获取预授权码 | `POST https://api.weixin.qq.com/cgi-bin/component/api_create_preauthcode?access_token=ACCESS_TOKEN` | component_access_token | `component_appid` | `pre_auth_code`, `expires_in` |
| 获取刷新令牌（授权码换 refresh_token） | `POST https://api.weixin.qq.com/cgi-bin/component/api_query_auth?access_token=ACCESS_TOKEN` | component_access_token | `component_appid`, `authorization_code` | `authorization_info{...}` |
| 获取/刷新授权方令牌 | `POST https://api.weixin.qq.com/cgi-bin/component/api_authorizer_token?access_token=ACCESS_TOKEN` | component_access_token | `component_appid`, `authorizer_appid`, `authorizer_refresh_token` | `authorizer_access_token`, `expires_in`, `authorizer_refresh_token` |

> 旧版路径文档使用 `?component_access_token=COMPONENT_ACCESS_TOKEN` 作为查询参数名
> （见 [预授权码 2.0](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/ThirdParty/token/pre_auth_code.html)：
> 「`component_access_token` string 是 第三方平台 component_access_token，**不是 authorizer_access_token**」）；
> 新版文档统一写作 `?access_token=ACCESS_TOKEN`，并在参数表注明「可使用 component_access_token」。
> **两个参数名微信侧都接受**，实现时建议按新版文档用 `access_token`。

---

## 2. 预授权与授权

### 2.1 api_create_preauthcode（获取预授权码）

| 项 | 值 |
|---|---|
| URL | `POST https://api.weixin.qq.com/cgi-bin/component/api_create_preauthcode?access_token=ACCESS_TOKEN` |
| Method | `POST`（JSON body） |
| 鉴权 | **component_access_token** |
| 接口英文名 | `getPreAuthCode` |

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `access_token` | string | 是 | 查询参数，可用 `component_access_token` |
| `component_appid` | string | 是 | 第三方平台 appid |

返回：

| 参数 | 类型 | 说明 |
|---|---|---|
| `pre_auth_code` | string | 预授权码 |
| `expires_in` | number | 有效期，单位：秒 |

```json
{ "pre_auth_code": "Cx_Dk6qiBE0Dmx4EmlT3oRfArPvwSQ-oa3NL_fwHM7VI08r52wazoZX2Rhpz1dEw", "expires_in": 600 }
```

**有效期与使用次数**

- 正文原文：「每个预授权码有效期为 **1800 秒**」；
- 但同页返回示例给出 `"expires_in": 600`，旧版 2.0 文档示例同样是 600 —— **文档自相矛盾**。
- 实现建议：**以响应里的 `expires_in` 为准**。→ 具体秒数标注「文档不一致（1800 秒 vs 示例 600）」。
- **使用次数：官方文档未明确说明**（未见"只能用一次"之类表述）。→ 标注**文档未明确**。

错误码：`-1`、`40001`、`40013`、`40125`、`41001 access_token missing`、`41004`、`42001 access_token expired`、`45009`、`47001`、`48001`、`61004`、`61005`、`61006`、`61011`。

来源：[获取预授权码](https://developers.weixin.qq.com/doc/oplatform/openApi/ticket-token/api_getpreauthcode.html)、[预授权码（2.0 旧路径）](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/ThirdParty/token/pre_auth_code.html)

---

### 2.2 授权链接 / 授权二维码

来自「[授权流程技术说明](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/Authorization_Process_Technical_Description.html)」。
文档明确当前提供**三种授权方式**：PC 版扫码授权、H5 版授权、小程序插件版授权；「访问 PC 版授权链接则会**自动出现授权码（二维码）**，商家以微信扫码的方式进入授权账号列表页」。

**授权链接参数（通用）**

| 参数 | 必填 | 说明 |
|---|---|---|
| `component_appid` | 是 | 第三方平台方 appid |
| `pre_auth_code` | 是 | 预授权码 |
| `redirect_uri` | 是 | 授权回调 URI，格式为 `https://xxx`（**插件版无该参数**）。管理员授权确认后自动跳转，URL 参数返回 `auth_code` 与 `expires_in`：`redirect_url?auth_code=xxx&expires_in=600` |
| `auth_type` | 是 | 要授权的账号类型：`1` 手机端仅展示公众号；`2` 仅展示小程序；`3` 公众号和小程序都展示；`4` 小程序推客账号；`5` 视频号账号；`6` 全部（公众号、小程序、视频号）；`8` 带货助手账号 |
| `biz_appid` | 否 | 指定授权唯一的小程序或公众号。指定后只有该 appid 的管理员可授权，其他人扫码报错。与 `auth_type` 冲突时 **`biz_appid` 优先级更高** |
| `category_id_list` | 否 | 指定的权限集 id 列表；不填则默认拉取当前第三方账号已**全网发布**的权限集列表。单个写法 `category_id_list=99`，多个用中竖线 `\|` 分隔 |

**拼接方式（官方表格原文）**

| 版本 | URL |
|---|---|
| PC 版 | `https://mp.weixin.qq.com/cgi-bin/componentloginpage?component_appid=xxxx&pre_auth_code=xxxxx&redirect_uri=xxxx&auth_type=xxx` |
| H5 版-新版 | `https://open.weixin.qq.com/wxaopen/safe/bindcomponent?action=bindcomponent&no_scan=1&component_appid=xxxx&pre_auth_code=xxxxx&redirect_uri=xxxx&auth_type=xxx&biz_appid=xxxx#wechat_redirect` |
| H5 版-旧版（仅英文版文档保留） | `https://mp.weixin.qq.com/safe/bindcomponent?action=bindcomponent&no_scan=1&component_appid=xxxx&pre_auth_code=xxxxx&redirect_uri=xxxx&auth_type=xxx&biz_appid=xxxx#wechat_redirect` |
| 插件版 | 小程序插件 `miniprogram-thirdparty-plugin`，见下 |

**⚠️ 关于 `https://mp.weixin.qq.com/mp/authorize?...`**
在中文/英文、新版/旧版授权文档中**均未出现**该形式；官方「移动端授权」使用的是上表 **H5 版**链接。
→ 标注：**该 URL 形式未在官方文档中收录**（避免臆造）。

**插件版（需先申请「服务商组件」）**

```js
const MiniprogramThirdpartyPlugin = requirePlugin('miniprogram-thirdparty-plugin')
MiniprogramThirdpartyPlugin.init(wx)
MiniprogramThirdpartyPlugin.openAuthorizeAccount({
  platformAppID: '',   // 第三方平台方 appid（必填）
  preAuthCode: '',     // 预授权码（必填）
  authType: 3,         // 1 仅公众号 / 2 仅小程序 / 3 都展示（必填）
  bizAppid: 'wxxxxxxxxx' // 非必填
})
```

**授权回调（关键）**

> 「管理员授权确认之后，授权页会自动跳转进入回调 URI，并在 URL 参数中返回**授权码和过期时间**：`redirect_url?auth_code=xxx&expires_in=600`。」

> 授权前提醒：如果该第三方平台账号**尚未全网发布**，需要先把用于测试的公众号/小程序加入「第三方平台-开发资料」的「**授权测试公众号/小程序列表**」。

---

### 2.3 api_query_auth（授权码换 authorizer_access_token + authorizer_refresh_token）

| 项 | 值 |
|---|---|
| URL | `POST https://api.weixin.qq.com/cgi-bin/component/api_query_auth?access_token=ACCESS_TOKEN` |
| Method | `POST`（JSON body） |
| 鉴权 | **component_access_token** |
| 接口英文名 | `getAuthorizerRefreshToken` |

**请求体**

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `component_appid` | string | 是 | 第三方平台 appid |
| `authorization_code` | string | 是 | 授权码，授权成功时返回给第三方平台；也可通过平台推送的「授权变更通知」获取 |

**返回体**

| 参数名 | 类型 | 说明 |
|---|---|---|
| `authorization_info` | object | 授权信息 |

`Res.authorization_info` Object：

| 参数名 | 类型 | 说明 |
|---|---|---|
| `authorizer_appid` | string | 授权的公众号或者小程序 appid |
| `authorizer_access_token` | string | 接口调用令牌（在授权的公众号/小程序**具备 API 权限时**才有此返回值） |
| `expires_in` | number | authorizer_access_token 有效期，单位秒（示例 7200） |
| `authorizer_refresh_token` | string | 刷新令牌（在授权的公众号**具备 API 权限时**才有此返回值）；一旦丢失只能让用户重新授权；用户重新授权后**之前的刷新令牌会失效** |
| `func_info` | objarray | 授权给第三方平台的权限集 id 列表 |

`Res.authorization_info.func_info(Array)`：

| 参数名 | 类型 | 说明 |
|---|---|---|
| `funcscope_category` | object | 授权给开发者的权限集详情 |

`...funcscope_category` Object：`id`(number, 权限集id) / `type`(number) / `name`(string) / `desc`(string)
（注：2.0 旧路径文档给出 4 个字段，新版文档只列 `id`；实际返回通常只含 `id`，示例中均为 `{"funcscope_category":{"id":1}}`。）

```json
{
  "authorization_info": {
    "authorizer_appid": "wxf8b4f85f3a794e77",
    "authorizer_access_token": "QXjUqNqfYVH0yBE1iI_7vuN_9gQbpjfK7hYwJ3P7xOa88a89-...",
    "expires_in": 7200,
    "authorizer_refresh_token": "dTo-YCXPL4llX-u1W1pPpnp8Hgm4wpJtlR6iV0doKdY",
    "func_info": [{ "funcscope_category": { "id": 1 } }, { "funcscope_category": { "id": 2 } }]
  }
}
```

**注意事项（原文）**：公众号/小程序可自定义选择部分权限授权，所以**必须用该接口判断实际授权了哪些权限**，不能假设自己声明的权限就是对方授权的权限。

错误码：`-1`、`0 ok`、`40001`、`40013`。

---

### 2.4 api_authorizer_token（刷新授权方令牌）

| 项 | 值 |
|---|---|
| URL | `POST https://api.weixin.qq.com/cgi-bin/component/api_authorizer_token?access_token=ACCESS_TOKEN` |
| Method | `POST`（JSON body） |
| 鉴权 | **component_access_token**（⚠️ 不是 authorizer_access_token；传错会 `61014`） |
| 接口英文名 | `getAuthorizerAccessToken` |

**请求体**

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `component_appid` | string | 是 | 第三方平台 appid |
| `authorizer_appid` | string | 是 | 授权方 appid |
| `authorizer_refresh_token` | string | 是 | 刷新令牌，获取授权信息时得到 |

**返回体**

| 参数名 | 类型 | 说明 |
|---|---|---|
| `authorizer_access_token` | string | 授权方令牌 |
| `expires_in` | number | 有效期，单位：秒（示例 7200） |
| `authorizer_refresh_token` | string | 刷新令牌 |

**注意（原文）**：
> 「authorizer_access_token 有效期为 2 小时，开发者需要缓存 authorizer_access_token，避免获取/刷新接口调用令牌的 API 调用触发每日限额。」

**refresh_token 是否变化**：响应中**始终返回**一个 `authorizer_refresh_token`。
官方文档**没有明确说刷新后 refresh_token 是否一定变化**；[creat_token 页面的 FAQ](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/creat_token.html) 只说
「解除授权 → 重新授权会改变 authorizer_refresh_token」。
→ 实现建议：**每次刷新后都覆盖保存返回的 refresh_token**（这是唯一安全的做法），但注意"是否变化"官方**未明确**。

错误码：`-1`、`0 ok`、`40001`、`40013`、`61014 must use component token for component api`。

---

### 2.5 授权事件推送（authorized / updateauthorized / unauthorized）

| 项 | 内容 |
|---|---|
| 推送目标 | 第三方平台的「**授权事件接收 URL**」（创建第三方平台时填写） |
| 方式 | `POST` |
| 必须响应 | **直接返回字符串 `success`**（明文） |
| 加密 | 安全模式，**加密**（XML 中 `Encrypt` 字段）。见[第 6 节](#6-加解密规范wxbizmsgcrypt) |

**解密后字段说明**

| 参数 | 类型 | 字段描述 |
|---|---|---|
| `AppId` | string | 第三方平台 appid |
| `CreateTime` | number | 时间戳 |
| `InfoType` | string | 通知类型 |
| `AuthorizerAppid` | string | 公众号/服务号/小程序/微信小店/带货助手/视频号助手的 appid |
| `AuthorizationCode` | string | 授权码，可用于获取授权信息 |
| `AuthorizationCodeExpiredTime` | number | 授权码过期时间，单位秒 |
| `PreAuthCode` | string | 预授权码 |

**InfoType 说明**

| type | 说明 |
|---|---|
| `authorized` | 授权成功 |
| `updateauthorized` | 更新授权 |
| `unauthorized` | 取消授权 |

**推送内容解密后的示例（原文）**

```xml
<!-- 授权成功通知 -->
<xml>
  <AppId>第三方平台appid</AppId>
  <CreateTime>1413192760</CreateTime>
  <InfoType>authorized</InfoType>
  <AuthorizerAppid>公众号appid</AuthorizerAppid>
  <AuthorizationCode>授权码</AuthorizationCode>
  <AuthorizationCodeExpiredTime>过期时间</AuthorizationCodeExpiredTime>
  <PreAuthCode>预授权码</PreAuthCode>
</xml>

<!-- 取消授权通知 -->
<xml>
  <AppId>第三方平台appid</AppId>
  <CreateTime>1413192760</CreateTime>
  <InfoType>unauthorized</InfoType>
  <AuthorizerAppid>公众号appid</AuthorizerAppid>
</xml>

<!-- 授权更新通知 -->
<xml>
  <AppId>第三方平台appid</AppId>
  <CreateTime>1413192760</CreateTime>
  <InfoType>updateauthorized</InfoType>
  <AuthorizerAppid>公众号appid</AuthorizerAppid>
  <AuthorizationCode>授权码</AuthorizationCode>
  <AuthorizationCodeExpiredTime>过期时间</AuthorizationCodeExpiredTime>
  <PreAuthCode>预授权码</PreAuthCode>
</xml>
```

> 补充说明（原文）：「如果更新授权时，授权的权限集**没有发生变化**，将**不会触发**授权更新通知。」
> 注意：授权变更通知**没有 JSON 版本**，第三方平台固定 XML（视频号小店等场景的 JSON 回包见第 6 节说明）。

来源：[授权变更通知推送](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/authorize_event.html)

---

## 3. 授权方信息

### 3.1 api_get_authorizer_info

| 项 | 值 |
|---|---|
| URL | `POST https://api.weixin.qq.com/cgi-bin/component/api_get_authorizer_info?access_token=ACCESS_TOKEN` |
| Method | `POST`（JSON body） |
| 鉴权 | **component_access_token** |
| 接口英文名 | `getAuthorizerInfo` |

**请求体**：`component_appid`(是), `authorizer_appid`(是)

**返回体（顶层）**

| 参数名 | 类型 | 说明 |
|---|---|---|
| `authorizer_info` | object | 授权账号信息 |
| `authorization_info` | object | 授权信息 |

**`authorizer_info` 字段**

| 参数名 | 类型 | 说明 |
|---|---|---|
| `nick_name` | string | 昵称（⚠️ 官方字段名是 **`nick_name`**，不是 `nickname`） |
| `head_img` | string | 头像 |
| `service_type_info` | object | 公众号/小程序类型 |
| `verify_type_info` | object | 公众号/小程序认证类型 |
| `user_name` | string | 原始 ID（gh_xxx） |
| `alias` | string | 公众号所设置的微信号，可能为空 |
| `qrcode_url` | string | 二维码图片的 URL |
| `business_info` | object | 用以了解功能的开通状况（0 未开通，1 已开通） |
| `idc` | number | 废弃参数 |
| `principal_name` | string | 主体名称 |
| `signature` | string | 小程序账号介绍 |
| `MiniProgramInfo` | object | 小程序配置信息，**可根据这个字段判断是否为小程序类型授权** |
| `register_type` | number | 小程序注册方式（枚举） |
| `account_status` | number | 账号状态，该字段小程序也返回（枚举） |
| `basic_config` | object | 基础配置信息 |
| `channels_info` | object | 视频号账号类型；该授权账号为视频号时返回 |
| `store_info` | object | 小店账号类型；为小店时返回 |
| `talent_info` | object | 带货助手账号类型；为带货助手时返回 |
| `supplier_info` | object | 供货商账号类型；为供货商时返回 |

> 变更日志：2026-03-17 新增 `store_info`/`talent_info`/`supplier_info`；2025-11-25 上述字段 `id` 新增枚举值。

**子对象**

- `service_type_info` = `{ id: number, name: string }`
- `verify_type_info` = `{ id: number, name: string }`
- `business_info` = `{ open_pay, open_shake, open_scan, open_card, open_store }`（number，0/1）
- `MiniProgramInfo` = `{ network: object, categories: objarray, visit_status: number(废弃) }`
  - `network` = `{ RequestDomain:[], WsRequestDomain:[], UploadDomain:[], DownloadDomain:[], UDPDomain:[], TCPDomain:[] }`
  - `categories[]` = `{ first: string(一级类目), second: string(二级类目) }`
  - （示例中另出现过 `BizDomain:[]`，字段表未列出 → 文档未明确）
- `basic_config` = `{ is_phone_configured: boolean, is_email_configured: boolean }`
- `channels_info` / `store_info` / `talent_info` / `supplier_info` = `{ id: number }`

**`authorization_info` 字段**

| 参数名 | 类型 | 说明 |
|---|---|---|
| `authorizer_appid` | string | 授权的公众号或者小程序 appid |
| `authorizer_refresh_token` | string | 刷新令牌（授权的公众号具备 API 权限时才有此返回值） |
| `func_info` | objarray | 授权给第三方平台的权限集 id 列表，元素为 `{ funcscope_category: { id: number } }` |

> 注意：与本接口**不返回** `authorizer_access_token`（这是与 `api_query_auth` 的区别）。

**枚举**

`service_type_info.id`：

| 值 | 描述 |
|---|---|
| 0 | 订阅号 / 普通小程序（文档表格中 0 同时对应两者） |
| 1 | 由历史老账号升级后的订阅号 |
| 2 | 服务号 |
| 12 | 试用小程序 |
| 4 | 小游戏 |
| 10 | 小商店 |
| 2 或者 3 | 门店小程序 |

`verify_type_info.id`：

| 值 | 描述 |
|---|---|
| -1 | 未认证 |
| 0 | 微信认证 |
| 1 | 新浪微博认证 |
| 3 | 已资质认证通过但还未通过名称认证 |
| 4 | 已资质认证通过、还未通过名称认证，但通过了新浪微博认证 |

`register_type`（小程序注册方式）：
`0` 普通注册 / `2` 复用公众号创建小程序 api 注册 / `6` 法人扫脸创建企业小程序 api 注册 / `13` 创建试用小程序 api 注册 / `15` 联盟控制台注册 / `16` 创建个人小程序 api 注册 / `17` 创建个人交易小程序 api 注册 / `19` 试用小程序转正 api 注册 / `22` 复用商户号创建企业小程序 api 注册 / `23` 复用商户号转正 api 注册

`account_status`：`1` 正常 / `14` 已注销 / `16` 已封禁 / `18` 已告警 / `19` 已冻结
`channels_info.id`：`0` 普通视频号；`store_info.id`：`0` 普通小店；`talent_info.id`：`0` 普通带货助手账号；`supplier_info.id`：`0` 普通供货商账号

**示例（小程序授权）**

```json
{"authorizer_info":
  {"nick_name":"找呀找呀找盲盒","head_img":"http:xxx",
   "service_type_info":{"id":0},"verify_type_info":{"id":-1},
   "user_name":"gh_3dacad47dc6b","alias":"","qrcode_url":"http:xxx",
   "business_info":{"open_pay":0,"open_shake":0,"open_scan":0,"open_card":0,"open_store":0},
   "idc":1,"principal_name":"个人","signature":"...",
   "MiniProgramInfo":{"network":{"RequestDomain":["https:xxx"],"WsRequestDomain":[],"UploadDomain":[],"DownloadDomain":[],"BizDomain":[],"UDPDomain":[]},
                      "categories":[{"first":"工具","second":"效率"}],"visit_status":0}},
 "authorization_info":{"authorizer_appid":"wxf24d2dfc1a974128",
   "authorizer_refresh_token":"xxxxxx",
   "func_info":[{"funcscope_category":{"id":17}},{"funcscope_category":{"id":18}}]}}
```

错误码：`-1`、`0`、`40001`、`40013`。

---

### 3.2 api_get_authorizer_list（拉取已授权账号列表）

| 项 | 值 |
|---|---|
| URL | `POST https://api.weixin.qq.com/cgi-bin/component/api_get_authorizer_list?access_token=ACCESS_TOKEN` |
| Method | `POST`（JSON body） |
| 鉴权 | **component_access_token** |
| 接口英文名 | `getAuthorizerList` |

**请求体**

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `component_appid` | string | 是 | 第三方平台 APPID |
| `offset` | number | 是 | 偏移位置/起始位置 |
| `count` | number | 是 | 拉取数量，**最大为 500** |

**返回体**

| 参数名 | 类型 | 说明 |
|---|---|---|
| `total_count` | number | 授权的账号总数 |
| `list` | objarray | 当前查询的帐号基本信息列表 |

`list[]` Object：`authorizer_appid`(string) / `refresh_token`(string, 即 authorizer_refresh_token) / `auth_time`(number, 授权时间)

```json
{ "total_count": 33,
  "list": [ { "authorizer_appid": "authorizer_appid_1", "refresh_token": "refresh_token_1", "auth_time": 1558000607 } ] }
```

错误码：`-1`、`0`、`40001`、`40013`、`40170 args count exceed count limit`（个数超出限制）。

> ⚠️ 该接口**不返回** authorizer_access_token（也没有像 access_token 那样可能返回空值的说明）。文档中「authorizer_refresh_token 返回空的情况」出现在 creat_token FAQ，指其他场景。

---

### 3.3 api_get_authorizer_option / api_set_authorizer_option

| 接口 | Method + URL | 鉴权（按文档） |
|---|---|---|
| 获取授权方选项信息 | `POST https://api.weixin.qq.com/cgi-bin/component/get_authorizer_option?access_token=ACCESS_TOKEN` | 中文页：「可使用 `component_access_token`」；**英文页写「use authorizer_access_token」（⚠️ 文档自相矛盾）** |
| 设置授权方选项信息 | `POST https://api.weixin.qq.com/cgi-bin/component/set_authorizer_option?access_token=ACCESS_TOKEN` | 同上（中英矛盾） |
| 接口英文名 | `getAuthorizerOptionInfo` / `setAuthorizerOptionInfo` | |

**Get 请求体**

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `option_name` | string | 是 | 选项名称 |

**Get 返回体**：`option_name`(string), `option_value`(string)

**Set 请求体**：`option_name`(string, 是), `option_value`(string, 是)
**Set 返回体**：`errcode`(number), `errmsg`(string)

**⚠️ `authorizer_appid` 的位置**
新版文档的「请求体 Request Payload」表**只列了 `option_name` / `option_value`**，未列 `authorizer_appid` / `component_appid`。
但全局错误码中存在 `41018 missing component_appid`，且历史文档与本接口语义都要求 `component_appid` + `authorizer_appid`。
→ 标注：**官方新版文档字段表缺失，实际请求通常需要 `component_appid` 与 `authorizer_appid`；请以实际联调结果为准。**（这是本项目中需要重点实测的一处。）

**option_name / option_value 说明（官方原文表格）**

| option_name | 选项名说明 | option_value | 选项值说明 |
|---|---|---|---|
| `location_report` | 地理位置上报选项 | `0` | 无上报 |
| | | `1` | 进入会话时上报 |
| | | `2` | 每 5s 上报 |
| `voice_recognize` | 语音识别开关选项 | `0` | 关闭语音识别 |
| | | `1` | 开启语音识别 |
| `customer_service` | 多客服开关选项 | `0` | 关闭多客服 |
| | | `1` | 开启多客服 |

**小程序相关选项**：官方 `get/set_authorizer_option` 文档**只列出上述三个选项**（均为公众号时代的选项），
**未列出任何小程序专属 option_name**。→ 标注：**文档未明确**（不要臆造 `wx_*` 之类的选项名）。

Set 接口注意事项（原文）：「设置各项选项设置信息，需要有授权方的授权，详见权限集说明。」

错误码：Get：`0`、`40001`；Set：`40013`。
（旧版 2.0 文档中 Get 接口的 URL 写作 `.../component/get_authorizer_option`、Set 写作 `.../component/set_authorizer_option`；任务书中的 `api_get_authorizer_option` / `api_set_authorizer_option`
是**接口英文名式**的写法，微信实际 URL 路径**不带 `api_` 前缀**。）

来源：[获取授权方选项信息](https://developers.weixin.qq.com/doc/oplatform/openApi/authorization-management/api_getauthorizeroptioninfo.html)、[设置授权方选项信息](https://developers.weixin.qq.com/doc/oplatform/openApi/authorization-management/api_setauthorizeroptioninfo.html)

---

## 4. 小程序代码模板库（第三方平台侧）

> **全部 4 个接口都用 component_access_token，属"仅支持第三方平台自己调用"。**

| 接口 | Method + URL | Body | 返回 |
|---|---|---|---|
| 获取草稿箱列表 | `GET https://api.weixin.qq.com/wxa/gettemplatedraftlist?access_token=ACCESS_TOKEN` | 无 | `errcode`,`errmsg`,`draft_list[]` |
| 将草稿添加到模板库 | `POST https://api.weixin.qq.com/wxa/addtotemplate?access_token=ACCESS_TOKEN` | `draft_id`(number,是), `template_type`(number,否,默认0) | `errcode`,`errmsg` |
| 获取模板列表 | `GET https://api.weixin.qq.com/wxa/gettemplatelist?access_token=ACCESS_TOKEN` | `template_type`(number,否,0/1,不填返回全部) | `errcode`,`errmsg`,`template_list[]` |
| 删除代码模板 | `POST https://api.weixin.qq.com/wxa/deletetemplate?access_token=ACCESS_TOKEN` | `template_id`(number,是) | `errcode`,`errmsg` |

**草稿 `draft_list[]` 字段**：`create_time`(number, 开发者上传草稿时间戳) / `user_version`(string) / `user_desc`(string) / `draft_id`(number) /
`source_miniprogram_appid`(string, 开发小程序的 appid) / `source_miniprogram`(string, 开发小程序的名称) / `developer`(string, 操作者微信昵称)
（示例中还有 `category_list: []`，字段表未列）

**模板 `template_list[]` 字段**：`create_time` / `user_version` / `user_desc` / `template_id`(number) / `draft_id`(number) /
`source_miniprogram_appid` / `source_miniprogram` / `template_type`(0 普通模板，1 标准模板) /
`category_list`(标准模板的类目信息；普通模板为空数组；结构同 submit_audit 的 item：`address` `tag` `first_class` `second_class` `third_class` `title` `first_id` `second_id` `third_id`) /
`audit_scene`(标准模板场景标签，普通模板不返回) / `audit_status`(标准模板审核状态，普通模板不返回) / `reason`(标准模板驳回原因，普通模板不返回)

`audit_status` 枚举：`0` 未提审核 / `1` 审核中 / `2` 审核驳回 / `3` 审核通过 / `4` 提审中 / `5` 提审失败

**⚠️ 注意事项（原文）**：`gettemplatelist` 「请求方式是 **get，不是 post**。如果之前使用了 post 请求的用户，请切换成 get。」（错误码 `43001 require GET method`）

### 4.1 模板库数量上限与「草稿 / 模板」的区别

官方「[小程序模板库管理](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/operation/thirdparty/template.html)」原文：

> **限制说明**
> 1）**小程序模板库的存储数量上限为 200 个。**
> 2）由于标准模板依赖交易组件，但交易组件已下架，因此建议开发者使用普通模板（**标准模板已无法继续使用**）。平台近期也会对标准模板进行下架处理。

官方「[服务商代开发小程序](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/product/how_to_dev.html)」原文：

> 「从开发者工具中上传的代码，会先存在**草稿箱**中，**每个开发小程序只保留最新一份上传记录**。开发者可将草稿箱中的代码添加到小程序模板库中，**小程序模板库中的模板不会被覆盖**。**最多可以有 200 个代码模板**，添加后可以获得模板 ID（TemplateID）。」

| 维度 | 草稿（draft） | 模板（template） |
|---|---|---|
| 来源 | 用微信开发者工具在「开发小程序」中点击「上传」后落库 | 调 `addtotemplate` 把某个 draft 加入模板库 |
| 唯一性 | **每个开发小程序只保留最新一份**（同一个小程序再次上传会覆盖草稿） | **不会被覆盖**，持久保存 |
| 数量上限 | 文档未给出明确数字 | **200 个** |
| 用途 | 中间态 | 供 `/wxa/commit` 的 `template_id` 使用 |
| 错误码 | `85064 template not found` | `85065 template list is full`（模板库已满） |

> 另：绑定为「开发小程序」后，该小程序在开发者工具中上传的代码会**直接上传到开放平台**，不会上传到公众平台。
> 另有 `directCommit`（ext.json 里的参数）可绕过模板库直接提交至待审核列表。

---

## 5. 为授权小程序上传代码 / 提审 / 发布

### 5.0 【重点】这些接口用哪个 token？要不要额外 `component_appid`？

**结论（依据官方文档每页的「第三方调用」小节与参数表）：**

1. `/wxa/*`（代码管理类）接口 **URL 中的 `access_token` 必须是 `authorizer_access_token`**（代商家调用）。
2. 依据全局错误码与本类接口文档，**不需要**再额外传 `component_appid` 参数。
   - 「[如何代商家调用接口](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/getting_started/how_to_call_api.html)」原文：
     「**公众号、服务号、小程序、微信小店、带货助手、视频号助手的 API，服务商都可以调用，只是服务商调用的时候要使用 `authorizer_access_token`，而不是 `access_token`。**」
   - 反例（**唯一需要额外 `component_appid` 的场景**）：`GET https://api.weixin.qq.com/sns/component/jscode2session?appid=APPID&grant_type=authorization_code&component_appid=COMPONENT_APPID&component_access_token=COMPONENT_ACCESS_TOKEN&js_code=JS_CODE`
     —— 该接口用 **component_access_token + component_appid**，见 [小程序登录（服务商）](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/login/api_thirdpartycode2session.html)。
3. 权限集要求：`/wxa/commit`、`/wxa/get_page`、`/wxa/submit_audit`、`/wxa/get_auditstatus`、`/wxa/get_latest_auditstatus`、`/wxa/undocodeaudit`、`/wxa/release`、`/wxa/revertcoderelease`、`/wxa/grayrelease`、`/wxa/getgrayreleaseplan`、`/wxa/revertgrayrelease`、`/wxa/change_visitstatus`、`/wxa/getvisitstatus`、`/cgi-bin/wxopen/getweappsupportversion`、`/cgi-bin/wxopen/setweappsupportversion`、`/wxa/queryquota`、`/wxa/speedupaudit`、`/wxa/uploadmedia`、`/wxa/security/get_code_privacy_info` 均标注 **权限集 id = 18**；
   `/wxa/get_qrcode` 标注 **权限集 id = 18、86**。
   权限集 18 =「小程序开发与数据分析」（授权后为避免代码版本互相覆盖，**小程序无法再通过公众平台发版本**）。

---

### 5.1 `/wxa/commit` — 上传代码并生成体验版

| 项 | 值 |
|---|---|
| URL | `POST https://api.weixin.qq.com/wxa/commit?access_token=ACCESS_TOKEN` |
| Method | `POST`（JSON body） |
| 鉴权 | **authorizer_access_token** |
| 权限集 | 18 |
| 接口英文名 | `commit` |

**请求体**

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `template_id` | number | 是 | 代码库中的代码模板 ID（通过 `getTemplateList` 获取） |
| `ext_json` | string | 是 | **字符串形式**的 JSON，用于控制 ext.json 配置文件内容 |
| `user_version` | string | 是 | 代码版本号，开发者可自定义（**长度不要超过 64 个字符**） |
| `user_desc` | string | 是 | 代码描述，开发者可自定义 |

**返回体**：`errcode`(number), `errmsg`(string)

**`ext_json` 精确格式（官方示例原文）**

```json
{
  "template_id": "0",
  "ext_json": "{\"extAppid\":\"\",\"ext\":{\"attr1\":\"value1\",\"attr2\":\"value2\"},\"extPages\":{\"index\":{},\"search/index\":{}},\"pages\":[\"index\",\"search/index\"],\"window\":{},\"networkTimeout\":{},\"tabBar\":{},\"plugin\":{}}",
  "user_version": "V1.0",
  "user_desc": "test"
}
```

任务书中给出的形式 `{"extAppid":"","ext":{},"extPages":{},"pages":[],"window":{},"tabBar":{},"networkTimeout":{},"debug":false,"subpackages":[]}`
与之基本一致；官方示例中出现的是 `plugin`（不是 `debug`），且分包键在小程序配置中是 `subPackages`/`subpackages`（官方 commit 文档正文只提「有限支持 subPackages」）。
`debug` 字段在官方 ext_json 文档中**未出现** → 标注：**文档未明确**。

**`ext_json` 合并规则（官方原文，逐条）**

- `ext` 整体替换
- `pages` 整体替换
- `extPages` 中找到对应页面，**同级覆盖** page.json
- `window` 同级覆盖
- `extAppid` 直接加到 app.json
- `networkTimeout` 同级覆盖
- `customOpen` 整体替换
- `tabbar` 同级覆盖
- `functionPages` 整体替换
- `subPackages` 整体替换
- `navigateToMiniProgaramAppIdList`：整体替换
- `plugins` 整体替换

> 「同级覆盖」= 遍历 extjson 的对象成员，app.json 中存在该成员则覆盖，不存在则添加。
> 「整体替换」= app.json 存在该对象就用 extjson 的覆盖，不存在则添加。

**约束**：
- `ext_json` 中**有限支持 `pages`**：只能配置模板页面的**子集**（不可新增页面）。
- `ext_json` 中**有限支持 `subPackages`**：只能配置模板分包及其页面的子集（分包必须已声明于模板中，且**不可新增分包页面**）。
- `ext_json` 支持 `plugins` 配置，会**覆盖**模板 app.json 中的 plugins 配置。
- 特殊字段：`ext`（自定义字段仅允许在这里定义，可在小程序中调用）/ `extPages`（页面配置）/ `extAppid`（授权方 Appid）/ `sitemap`。
- **标准模板**限制：若 `template_id` 属于标准模板库，`ext_json` 支持的参数仅为 `{"extAppid":'', "ext": {}, "window": {}}`（否则报 `9402203`）。

**`requiredPrivateInfos`（地理位置隐私接口声明）**

- 规则为「**整体替换**」：ext_json 里配了就以 ext_json 为准，覆盖 app.json 的配置。
- 示例：`{"template_id":"95","ext_json":"{\"requiredPrivateInfos\":[\"onLocationChange\",\"startLocationUpdate\"]}","user_version":"V1.0","user_desc":"test"}`
- 校验错误码：`85310`（格式/名称错误）、`85311`（包含互斥 api）、`85312`（配置了无权限的 api）。

**代码包大小限制**

- `/wxa/commit` 文档只说超限返回 `85044 package exceed max limit`，**没有在该页给出具体 MB 数**。
- 具体数字在[分包加载](https://developers.weixin.qq.com/miniprogram/dev/framework/subpackages.html)官方原文：
  > 「目前小程序分包大小有以下限制：
  > **整个小程序所有分包大小不超过 30M（服务商代开发的小程序不超过 20M）**
  > **单个分包/主包大小不能超过 2M**」
- 即：**主包/单个分包 ≤ 2MB**；代开发小程序 **总包 ≤ 20MB**（普通小程序 30MB）。任务书中的 "2MB 限制" 指**单个包**。

**其他注意事项（原文）**

- 如果接口涉及地理位置相关隐私接口，需要在 `ext_json` 中配置 `requiredPrivateInfos`。
- 上传代码后，**需要等检测任务结束后方可提交代码审核**，否则提交审核会报 `61039`。
- 可用 `getCodePrivacyInfo` 获取检测结果，确认检测任务结束后再提交审核。
- **不要**把「上传代码」和「提交代码审核」两个接口一起重试，否则会造成检测任务不断重启、一直 `61039`。
- 使用了地理位置接口但未在 ext_json 配置 → 提交审核报 `61040`；已配置但小程序未获对应 API 权限 → 也报 `61040`（可用 `privacy_api_not_use` 声明不使用）。

**错误码**

| 码 | 描述 |
|---|---|
| `-60005` | （文档表格未给描述） |
| `-1` | system error |
| `40001` / `40014` | token 无效 / 不合法的 access_token |
| `80066` | 非法的插件版本 |
| `80067` | 找不到使用的插件 |
| `80082` | 没有权限使用该插件 |
| `85013` | invalid ext_json, parse fail or containing invalid path（无效的自定义配置） |
| `85014` | template not exist（无效的模板编号） |
| `85043` | invalid template, something wrong?（模板错误） |
| `85044` | package exceed max limit（代码包超过大小限制） |
| `85045` | some path in ext_json not exist |
| `85046` | pagepath missing in tabbar list |
| `85047` | pages are empty |
| `85048` | parse ext_json fail |
| `85310` / `85311` / `85312` | requiredPrivateInfos 格式/名称错、互斥、无权限 |
| `9402202` | concurrent limit（请勿频繁提交，待上一次操作完成后再提交） |
| `9402203` | 标准模板 ext_json 错误（标准模板仅支持 `{"extAppid":'', "ext": {}, "window": {}}`） |

---

### 5.2 `/wxa/get_page` — 获取可配置的页面列表

| 项 | 值 |
|---|---|
| URL | `GET https://api.weixin.qq.com/wxa/get_page?access_token=ACCESS_TOKEN` |
| Method | `GET` |
| 鉴权 | **authorizer_access_token**（权限集 18） |
| 接口英文名 | `getCodePage` |

- 无请求体。
- 返回：`errcode`, `errmsg`, `page_list`(array，页面配置列表，元素为字符串路径)

```json
{ "errcode": 0, "errmsg": "ok", "page_list": ["index", "page/list", "page/detail"] }
```

- 错误码：`-1`、`0`、`40001`、`86000 should be called only from third party`、`86001 component experience version not exists`

---

### 5.3 `/wxa/submit_audit` — 提交代码审核

| 项 | 值 |
|---|---|
| URL | `POST https://api.weixin.qq.com/wxa/submit_audit?access_token=ACCESS_TOKEN` |
| Method | `POST`（JSON body） |
| 鉴权 | **authorizer_access_token**（权限集 18） |
| 接口英文名 | `submitAudit` |

**请求体**

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `item_list` | objarray | 是 | 审核项列表（**至多 5 项**）；类目是必填的，且要填写已经在小程序配置好的类目 |
| `feedback_info` | string | 否 | 反馈内容，至多 200 字 |
| `feedback_stuff` | string | 否 | 用 `\|` 分割的 media_id 列表，至多 5 张图片（通过新增临时素材接口上传得到） |
| `version_desc` | string | 否 | 小程序版本说明和功能解释 |
| `preview_info` | object | 否 | 预览信息（小程序页面截图和操作录屏） |
| `ugc_declare` | object | 否 | 用户生成内容场景（UGC）信息安全声明 |
| `privacy_api_not_use` | boolean | 否 | 声明是否不使用"代码中检测出但是未配置的隐私相关接口" |
| `order_path` | string | 否 | 订单中心 path |

**`item_list[]` 结构（精确字段）**

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `address` | string | 否 | 小程序的页面，可通过 `getCodePage` 获得 |
| `tag` | string | 否 | 小程序的标签，用空格分隔，**标签至多 10 个，标签长度至多 20** |
| `first_class` | string | 是 | 一级类目名称（`getAllCategoryName` 获取） |
| `second_class` | string | 是 | 二级类目名称 |
| `third_class` | string | 否 | 三级类目名称 |
| `title` | string | 否 | 小程序页面的标题，**标题长度至多 32** |
| `first_id` | number | 是 | 一级类目 id |
| `second_id` | number | 是 | 二级类目 id |
| `third_id` | number | 否 | 三级类目 id |

**`preview_info` 结构**：`video_id_list`(array, 录屏 mediaid 列表) / `pic_id_list`(array, 截屏 mediaid 列表)

**`ugc_declare` 结构**

| 字段 | 说明 |
|---|---|
| `scene` | UGC 场景：0 不涉及用户生成内容 / 1 用户资料 / 2 图片 / 3 视频 / 4 文本 / 5 音频（新版文档；旧文档写 `5其他`）**可多选，填 0 时无需填写下列字段** |
| `method` | 内容安全机制：1 使用平台建议的内容安全 API / 2 使用其他的内容审核产品 / 3 通过人工审核把关 / 4 未做内容审核把关 |
| `other_scene_desc` | 当 scene 选其他时的说明，不超过 256 字 |
| `has_audit_team` | 是否有审核团队，0 无 / 1 有，默认 0 |
| `audit_desc` | 说明当前对 UGC 内容的审核机制，不超过 256 字 |

**返回体**：`errcode`(number), `errmsg`(string), `auditid`(number, 审核编号)

**⚠️ 关于 "submit_audit/v2"、"auto_audit / auto_reject"**
- 官方文档（中文新版 `openApi/miniprogram-management/code-management/api_submitaudit.html`、旧版 `Third-party_Platforms/2.0/api/code/submit_audit.html`、英文版 `en/openApi/OpenApiDoc/miniprogram-management/code-management/submitAudit.html`）
  中**只有 `/wxa/submit_audit` 一个地址，没有 `/wxa/submit_audit/v2`**；请求体中**没有** `auto_audit` / `auto_reject` 字段。
- 多轮检索（含站内外的 `"submit_audit/v2"` 精确检索）**未找到官方 v2 接口文档**。
- → 结论：**官方文档未收录 `submit_audit/v2` 与自动提审参数**。若确有该能力，属未公开/灰度能力，请勿按此实现。**（标注：未能确认）**
- 「预审核 / 审核加速」官方提供的对应能力是：**加急代码审核** `/wxa/speedupaudit` + **查询服务商审核额度** `/wxa/queryquota`（见 5.7）。

**补充说明（原文）**

- 只有**上个版本被驳回**，才能使用 `feedback_info`、`feedback_stuff` 这两个字段，否则忽略处理。
- 当小程序**第一次提交审核**且类目包含「社交-社区/论坛、社交-笔记、社交-问答」其中之一时需填写 `ugc_declare`。
- 可另外调用「上传提审素材」接口，将截图/录屏上传并在提审时带上，帮助审核人员判断。
- 「提交代码审核的前置检查项」：**名称、简介、类目、头像 + 用户隐私保护指引**必须已配置；涉及地理位置等隐私接口还需申请权限并在代码中声明。
- 境外主体小程序需要补充「用户隐私保护指引」中「存储地区」信息，否则审核会被驳回（用 `setPrivacySetting` 配置 `store_region`）。
- 临时素材 mediaid 通过「临时素材管理接口」获取；**调用这些接口时必须使用 authorizer_access_token**。

**错误码（关键）**

| 码 | 说明 |
|---|---|
| `85006` 标签格式错误 | |
| `85007` 页面路径错误 | |
| `85008` category is in invalid format（当前小程序没有已经审核通过的类目） | |
| `85009` already submit a version under auditing（已经有正在审核的版本） | |
| `85010` item_list 有项目为空 | |
| `85011` 标题填写错误 | |
| `85023` item size is not in valid range（项目数不在 1-5 以内） | |
| `85051` data too large（version_desc 或 preview_info 超限） | |
| `85077` 小程序类目信息失效 | |
| `85085` submit audit reach limit（**小程序提审数量已达本月上限**） | |
| `85086` must commit before submit audit | |
| `85087` 已使用 navigateToMiniProgram，请声明跳转 appid 列表 | |
| `85092` invalid preview_info format / `85093` preview_info 视频或图片个数超限 / `85094` need add ugc declare | |
| `86000` 不是由第三方代小程序进行调用 / `86001` 不存在第三方已提交的代码 / `86002` 小程序还未设置昵称、头像、简介 | |
| `86007` 小程序禁止提交 / `86009` 服务商新增提审能力被限制 / `86010` 服务商迭代提审能力被限制 | |
| `87006` 小游戏不能提交 | |
| `61039` 隐私接口检查任务未完成，请稍等一分钟再重试 | |
| `61040` ext.json 配置的隐私接口无权限 / 代码中含有未配置的隐私接口 | |
| `9400001` 开发小程序已开通小程序直播权限，不支持发布版本 | |
| `9402202` concurrent limit | |

---

### 5.4 `/wxa/get_auditstatus` 与 `/wxa/get_latest_auditstatus`

| 接口 | Method + URL | Body | 鉴权 |
|---|---|---|---|
| 查询审核单状态 | `POST https://api.weixin.qq.com/wxa/get_auditstatus?access_token=ACCESS_TOKEN` | `{"auditid": 1234567}` | authorizer_access_token（权限集 18） |
| 查询最新一次审核单状态 | `GET https://api.weixin.qq.com/wxa/get_latest_auditstatus?access_token=ACCESS_TOKEN` | 无 | authorizer_access_token（权限集 18） |

**get_auditstatus 返回**：`errcode`, `errmsg`, `status`(number), `reason`(string, **status=1 时为拒绝原因；status=4 时为延后原因**), `screenshot`(string, status=1 时返回审核失败的截图示例，用竖线分隔的 media_id 列表，可通过获取永久素材接口拉取)
**错误码**：`85012 invalid audit id`、`86000`、`86001`

**get_latest_auditstatus 返回**：`errcode`, `errmsg`, `auditid`(number, 最新的审核 id), `status`(number), `reason`(string, 审核被拒绝时的原因), `screenshot`(string), `user_version`(string), `user_desc`(string), `submit_audit_time`(number, 提交审核的时间戳)

**审核状态 status 取值（两页一致，官方原文）**

| 状态值 | 说明 |
|---|---|
| `0` | 审核成功 |
| `1` | 审核被拒绝 |
| `2` | 审核中 |
| `3` | 已撤回 |
| `4` | 审核延后 |

---

### 5.5 `/wxa/release` 与 `/wxa/revertcoderelease`

**发布已通过审核的小程序**

| 项 | 值 |
|---|---|
| URL | `POST https://api.weixin.qq.com/wxa/release?access_token=ACCESS_TOKEN` |
| Method | `POST`（**body 必须传 `{}`**） |
| 鉴权 | **authorizer_access_token**（权限集 18） |
| 请求体 | **无字段**，但必须 POST 一个空 JSON |

> ⚠️ 官方注意事项原文：「注意，**post 的 data 为空，不等于不需要传 data**，否则会报错 `{"errcode": 44002, "errmsg": "empty post data"}`」。
> 请求示例即为 `{}`。

**返回**：`errcode`, `errmsg`
**错误码**：`-1`、`0`、`40001`、`40014`、`85019 no version is under auditing`（没有审核版本）、`85020 status not allowed`（审核状态未满足发布）

> 「干系人 / 审核通过后才能发布」：文档逻辑即「发布**最后一个审核通过**的代码版本」；若无可审核通过版本返回 `85019`/`85020`。
> 「干系人」概念在官方第三方平台发布文档中**未出现** → **文档未明确**。

**小程序版本回退**

| 项 | 值 |
|---|---|
| URL | `GET https://api.weixin.qq.com/wxa/revertcoderelease?access_token=ACCESS_TOKEN[&action=get_history_version][&app_version=123]` |
| Method | `GET` |
| 鉴权 | authorizer_access_token（权限集 18） |

- URL 参数：`action`（只能填 `get_history_version`，表示获取可回退的小程序版本；**该参数为 URL 参数，非 Body 参数**）、`app_version`（默认回滚到上一个版本；也可指定版本，**该参数为 URL 参数**）
- 返回：`errcode`, `errmsg`, `version_list`（**仅当 `action=get_history_version` 时返回**）
- `version_list[]`：`app_version`(number) / `user_version`(string) / `user_desc`(string) / `commit_time`(number)
- 注意事项：如果没有上一个线上版本，将无法回退；**最多保存最近发布或回退的 5 个版本**；当前版本回退后，**不能再调用版本回退接口，也不会再查到版本信息**
- 错误码：`40097 invalid args`、`87012 forbid revert this version release`（1 无上一个线上版；2 此版本为已回退版本；3 此版本为回退功能上线之前的版本）

---

### 5.6 `/wxa/getversioninfo` — 查询小程序版本信息

| 项 | 值 |
|---|---|
| URL | `POST https://api.weixin.qq.com/wxa/getversioninfo?access_token=ACCESS_TOKEN` |
| Method | `POST`（**body 必须传空 JSON `{}`**，官方："要传空的 json，不传会报错"） |
| 鉴权 | authorizer_access_token（权限集 18） |

**返回**：`errcode`, `errmsg`, `exp_info`(object, 体验版信息), `release_info`(object, 线上版信息)
- `exp_info`：`exp_time`(number, 提交体验版的时间) / `exp_version`(string) / `exp_desc`(string)
- `release_info`：`release_time`(number, 发布线上版的时间) / `release_version`(string) / `release_desc`(string)

**错误码**：`-1`、`0`、`44002 empty post data`
（→ `get_release_status` 这个接口名**不存在**；官方等价能力就是 `/wxa/getversioninfo`。）

---

### 5.7 审核撤回 / 加急 / 额度

| 接口 | Method + URL | Body | 返回 | 限制 |
|---|---|---|---|---|
| 撤回代码审核 | `GET https://api.weixin.qq.com/wxa/undocodeaudit?access_token=ACCESS_TOKEN` | 无 | `errcode`,`errmsg` | **单个账号每天审核撤回次数最多不超过 5 次**（每天额度从 0 点开始生效），**一个月不超过 10 次**；超限 `87013 no quota to undo code` |
| 加急代码审核 | `POST https://api.weixin.qq.com/wxa/speedupaudit?access_token=ACCESS_TOKEN` | `auditid`(number, 是) | `errcode`,`errmsg` | 加急后预计 **2-12 小时**内审完；错误码 `89401`~`89405`（`89405` 本月加急额度已用完） |
| 查询服务商审核额度 | `GET https://api.weixin.qq.com/wxa/queryquota?access_token=ACCESS_TOKEN` | 无 | `errcode`,`errmsg`,`rest`(quota 剩余值),`limit`(当月分配 quota),`speedup_rest`(剩余加急次数),`speedup_limit`(当月分配加急次数) | 所有旗下小程序**共用**该额度 |

三者鉴权均为 **authorizer_access_token**，权限集 18。

> 提审 quota 不足继续调用提交审核会报 `85085`；「[第三方小程序提审问题](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/troubleshooting/submit_quota.html)」指引可到「小程序服务商助手」小程序申请或咨询人工客服。

---

### 5.8 分阶段发布 / 服务状态 / 基础库版本

| 接口 | Method + URL | Body | 返回 |
|---|---|---|---|
| 分阶段发布 | `POST https://api.weixin.qq.com/wxa/grayrelease?access_token=ACCESS_TOKEN` | `gray_percentage`(number,是,**0~100 整数**；=0 时 `support_experiencer_first` 与 `support_debuger_first` 二选一必填), `support_debuger_first`(boolean,否,默认 false), `support_experiencer_first`(boolean,否,默认 false) | `errcode`,`errmsg` |
| 获取分阶段发布详情 | `GET https://api.weixin.qq.com/wxa/getgrayreleaseplan?access_token=ACCESS_TOKEN` | 无 | `errcode`,`errmsg`,`gray_release_plan{status,create_timestamp,gray_percentage,support_debuger_first,support_experiencer_first}` |
| 取消分阶段发布 | `GET https://api.weixin.qq.com/wxa/revertgrayrelease?access_token=ACCESS_TOKEN` | 无 | `errcode`,`errmsg` |
| 设置小程序服务状态 | `POST https://api.weixin.qq.com/wxa/change_visitstatus?access_token=ACCESS_TOKEN` | `action`(string,是, `close` 不可见 / `open` 可见) | `errcode`,`errmsg` |
| 查询小程序服务状态 | `POST https://api.weixin.qq.com/wxa/getvisitstatus?access_token=ACCESS_TOKEN` | 无（**要传空的 json `{}`**，不传报 `44002`） | `errcode`,`errmsg`,`status`(0 已暂停服务 / 1 未暂停服务) |
| 查询各版本用户占比 | `POST https://api.weixin.qq.com/cgi-bin/wxopen/getweappsupportversion?access_token=ACCESS_TOKEN` | 无（要传空 json） | `errcode`,`errmsg`,`now_version`,`uv_info.items[]{version,percentage}` |
| 设置最低基础库版本 | `POST https://api.weixin.qq.com/cgi-bin/wxopen/setweappsupportversion?access_token=ACCESS_TOKEN` | `version`(string,是, 已发布的基础库版本号) | `errcode`,`errmsg`（错误码 `89014 support version error`） |

`gray_release_plan.status`：`0` 初始状态 / `1` 执行中 / `2` 暂停中 / `3` 执行完毕 / `4` 被删除
`grayrelease` 错误码：`40097`、`85079`（没有线上版本）、`85080`（提交的审核未通过）、`85081`（无效的发布比例）、`85082`（当前比例需比之前设置的高）、`86002`
全部鉴权 **authorizer_access_token**，权限集 18。

---

### 5.9 体验版二维码 / 预览二维码

| 项 | 值 |
|---|---|
| URL | `GET https://api.weixin.qq.com/wxa/get_qrcode?access_token=ACCESS_TOKEN&path=PATH` |
| Method | `GET` |
| 鉴权 | 参数表写「**可使用 access_token、authorizer_access_token**」；第三方调用说明为权限集 **18、86**，代调用用 authorizer_access_token |
| 接口英文名 | `getTrialQRCode` |

- 请求体：`path`(string, 否) 指定二维码扫码后直接进入的页面并可带参数
- **`path` 需要进行一次 urlencode**：如 `page/index?action=1` 需填 `page%2Findex%3Faction%3D1`
- 正常时**直接返回二进制图片**（非 JSON），HTTP 头示例：

```
HTTP/1.1 200 OK
Connection: close
Content-Type: image/jpeg
Content-disposition: attachment; filename="QRCode.jpg"
Cache-Control: no-cache, must-revalidate
```

- 错误码：`-1`、`0`、`40001`、`40014`
- 实现提示：Go 侧需先读 `Content-Type`，若是 `image/*` 则落盘，若是 `application/json` 则解析错误码。

---

### 5.10 提审素材上传 / 隐私接口检测

**上传提审素材**

| 项 | 值 |
|---|---|
| URL | `POST https://api.weixin.qq.com/wxa/uploadmedia?access_token=ACCESS_TOKEN` |
| Method | `POST / FORM`（multipart） |
| 鉴权 | authorizer_access_token（权限集 18） |

- 表单字段：`media`
- 限制：**图片（image）2M**，支持 PNG/JPEG/JPG/GIF；**视频（video）10MB**，支持 MP4
- 返回：`errcode`, `errmsg`, `type`(string), `mediaid`(buffer)
- **返回的 mediaid 有效期是三天**，过期需重新上传
- 返回码：`43002`（需 POST）、`41005`（传输素材无视频或图片内容）、`40005`（格式不对）、`40006`（大小超限）、`86020`（小程序名称不合法）
- 调用示例：`curl -F media=@test.jpg "https://api.weixin.qq.com/wxa/uploadmedia?access_token=ACCESS_TOKEN"`

**获取隐私接口检测结果**

| 项 | 值 |
|---|---|
| URL | `GET https://api.weixin.qq.com/wxa/security/get_code_privacy_info?access_token=ACCESS_TOKEN` |
| Method | `GET` |
| 鉴权 | authorizer_access_token（权限集 18） |

- 返回：`errcode`, `errmsg`, `without_auth_list`(array, 没权限的隐私接口 api 英文名), `without_conf_list`(array, 没配置的隐私接口 api 英文名)
- 示例：`{"without_auth_list":["wx.getLocation","wx.onLocationChange"],"without_conf_list":["wx.onLocationChange"]}`
- 错误码：`61039`（检查任务未完成，请稍等一分钟再重试）、`61040`

---

### 5.11 审核状态推送（代码审核结果）

> 「当小程序有审核结果后，微信服务器会向第三方平台方的**消息与事件接收 URL**（创建第三方平台时填写）以 POST 的方式推送相关通知。
> **接收 POST 请求后，只需直接返回字符串 `success`。**」

**字段说明**

| 参数 | 类型 | 说明 |
|---|---|---|
| `ToUserName` | String | 小程序的原始 ID |
| `FromUserName` | String | 发送方账号（一个 OpenID，此时发送方是系统账号） |
| `CreateTime` | Number | 消息创建时间（整型），时间戳 |
| `MsgType` | String | 消息类型 `event` |
| `Event` | String | 事件类型 |
| `SuccTime` | Number | 审核成功时的时间戳 |
| `FailTime` | Number | 审核不通过的时间戳 |
| `DelayTime` | Number | 审核延后时的时间戳 |
| `Reason` | String | 审核不通过的原因 |
| `ScreenShot` | String | 审核不通过的截图示例。用 `\|` 分隔的 media_id 的列表，可通过获取永久素材接口拉取截图内容 |

**事件类型（⚠️ 注意：官方事件名是 `weapp_audit_*`，不是任务书中的 `wxa_audit_status`）**

| 事件类型 | 说明 |
|---|---|
| `weapp_audit_success` | 审核通过 |
| `weapp_audit_fail` | 审核不通过 |
| `weapp_audit_delay` | 审核延后 |

```xml
<!-- 审核通过 -->
<xml>
  <ToUserName><![CDATA[gh_fb9688c2a4b2]]></ToUserName>
  <FromUserName><![CDATA[od1P50M-fNQI5Gcq-trm4a7apsU8]]></FromUserName>
  <CreateTime>1488856741</CreateTime>
  <MsgType><![CDATA[event]]></MsgType>
  <Event><![CDATA[weapp_audit_success]]></Event>
  <SuccTime>1488856741</SuccTime>
</xml>

<!-- 审核不通过 -->
<xml>
  ... <Event><![CDATA[weapp_audit_fail]]></Event>
  <Reason><![CDATA[1:账号信息不符合规范:<br>...]]></Reason>
  <FailTime>1488856591</FailTime>
  <ScreenShot>xxx|yyy|zzz</ScreenShot>
</xml>

<!-- 审核延后 -->
<xml>
  ... <Event><![CDATA[weapp_audit_delay]]></Event>
  <Reason><![CDATA[...]]></Reason>
  <DelayTime>1488856591</DelayTime>
</xml>
```

> 该推送走「**消息与事件接收 URL**」（可含 `$APPID$` 占位符），与「授权事件接收 URL」（收 ticket 与授权变更通知）是**两个不同的 URL**。
> 除了消息通知之外，也可通过接口查询指定版本的审核状态、查询最新一次提交的审核状态。

---

## 6. 加解密规范（WXBizMsgCrypt）

### 6.1 消息推送的两个 URL 与校验参数

「[消息推送](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/message_push.html)」原文（第三方平台部分）：

登录微信开放平台，「管理中心」-「第三方平台」-「开发配置」-「开发资料」填写：

| 配置 | 说明 |
|---|---|
| **URL 服务器地址** | 必须以 `http://` 或 `https://` 开头，分别支持 **80 端口和 443 端口**；有两个 URL 配置 |
| ├ **授权事件接收配置** | 用于接收 `component_verify_ticket` 以及**授权变更通知**推送 |
| └ **消息与事件接收配置** | 推送给第三方平台或由第三方平台代收的消息与事件，**URL 中可以包含 `$APPID$`**，推送时会把代接收的小程序/公众号等的 APPID 填充在此 |
| **消息校验 Token** | 用于签名处理 |
| **消息加解密 Key** | 用作消息体加解密密钥（即 EncodingAESKey / symmetric_key） |
| **消息加解密方式** | 第三方平台**只允许安全模式**，不可选择（纯密文） |
| **数据格式** | 第三方平台**只允许为 XML**，不可选择 |

推送 URL 示例（含全部 query 参数）：

```
https://www.qq.com/wxba5fad812f8e6fb9/revice?signature=cc0c594499c1634947d5b502f158ee518947db27
   &timestamp=1715943329&nonce=1590219412&openid=o9AgO5Kd5ggOC-bXrbNODIiE3bGY
   &encrypt_type=aes&msg_signature=6c12a4205838198b8fa631b3220723bb07f1015c
```

> ⚠️ **关键**：官方原文明确 —— 「**校验 `msg_signature` 签名是否正确，以判断请求是否来自微信服务器。注意：不要使用 `signature` 验证！**」

推送包体（安全模式）：

```xml
<xml>
  <ToUserName><![CDATA[gh_97417a04a28d]]></ToUserName>
  <Encrypt><![CDATA[...base64...]]></Encrypt>
</xml>
```

### 6.2 签名算法

```
msg_signature = sha1( sort(Token, timestamp, nonce, msg_encrypt) )
```

- 官方原文（消息推送）：把 `token`、`timestamp`（URL 参数）、`nonce`（URL 参数）、`Encrypt`（**包体内的字段**）四个参数**进行字典序排序**，排序后**直接拼接成一个字符串**，然后 **sha1** 计算签名（**输出小写 hex**），与 URL 参数 `msg_signature` 对比。
- 官方原文（技术方案）：`msg_signature = sha1(sort(Token、timestamp、nonce, msg_encrypt))`
- 官方 PHP 实现（`sha1.php`）：

```php
$array = array($encrypt_msg, $token, $timestamp, $nonce);
sort($array, SORT_STRING);          // 字典序（字符串比较）
$str = implode($array);             // 直接拼接，无分隔符
return sha1($str);                  // 小写 hex
```

> Go 实现要点：`sort.Strings([]string{encrypt, token, timestamp, nonce})` → `strings.Join(..., "")` → `sha1.Sum` → `hex.EncodeToString`。

### 6.3 AES 细节

| 项 | 值（官方原文） |
|---|---|
| 算法 | **AES-256-CBC** |
| 密钥 | `AESKey = Base64_Decode(EncodingAESKey + "=")`，得到 **32 字节** |
| EncodingAESKey | 长度**固定 43 个字符**，从 `a-z, A-Z, 0-9` 共 62 个字符中选取 |
| IV | **`IV = AESKey 的前 16 字节`**（官方 PHP：`$iv = substr($this->key, 0, 16);`） |
| 填充 | **PKCS#7**，块大小 K = **32**：`Buf` 尾部填充 `(K - N%K)` 个字节，每个字节内容为 `(K - N%K)`（当 `N%K==0` 时填 32 个 `0x20` 值即 32 字节的 32） |
| Base64 | MIME 格式（`+`/`/`/`=`） |

```
PKCS#7 填充表（官方）
尾部填充        说明
01             if (N%K == (K-1))
0202           if (N%K == (K-2))
030303         if (N%K == (K-3))
...            ...
KK....KK       if (N%K == 0)     // K 个字节
```

官方 PHP `PKCS7Encoder`：

```php
public static $block_size = 32;
function encode($text) {
  $amount_to_pad = 32 - (strlen($text) % 32);
  if ($amount_to_pad == 0) { $amount_to_pad = 32; }
  return $text . str_repeat(chr($amount_to_pad), $amount_to_pad);
}
function decode($text) {
  $pad = ord(substr($text, -1));
  if ($pad < 1 || $pad > 32) { $pad = 0; }
  return substr($text, 0, strlen($text) - $pad);
}
```

> ⚠️ Go 的 `crypto/cipher` **没有内置 PKCS#7**，需自行实现；且填充块大小必须是 **32**，不能直接用 `aes.BlockSize`（16）——这是最容易踩的坑。

### 6.4 明文结构（FullStr）

```
FullStr = random(16B) + msg_len(4B) + msg + appid
msg_encrypt = Base64_Encode( AES_Encrypt( FullStr, AESKey ) )
```

| 组成 | 说明（官方原文） |
|---|---|
| `random(16B)` | 16 字节的随机字符串 |
| `msg_len(4B)` | `msg` 的长度，占 4 个字节（**网络字节序**，即 big-endian，PHP `pack("N", len)`） |
| `msg` | 明文消息体 |
| `appid` | **第三方平台的 appid**（代收用户消息时不是授权方的 appid；消息推送页原文：「appid 为第三方平台 Appid，**开发者需验证此 Appid 是否与自身第三方平台相符**」） |

官方给出的完整数值示例（"debug_demo" 事件，消息推送页）：
- `Encrypt` 密文 Base64 解码后 352 字节 → AES 解密后 `FullStr` 330 字节
- `random(16B)="1205899eaf019bbd"`（16 字符 = 16 字节）
- `msg_len=292`
- `appid="wx134c8103faa5a59e"`

### 6.5 解密流程（官方原文）

```
1. TmpMsg = Base64_Decode(msg_encrypt)
2. FullStr = AES_Decrypt(TmpMsg, AESKey)     // FullStr = random(16B) + msg_len(4B) + msg + appid
3. 验证尾部的 appid 是否正确（可选；文档另一处写"需验证"）
4. 去掉 FullStr 头部 16 字节的 random、4 字节的 msg_len、和尾部的 appid，即得到明文内容
```

官方 PHP `Prpcrypt::decrypt` 的**精确实现**（可直接对照写 Go）：

```php
$ciphertext_dec = base64_decode($encrypted);
$iv = substr($this->key, 0, 16);
$decrypted = mdecrypt_generic($module, $ciphertext_dec);   // AES-256-CBC 无 IV 之外的额外处理
$result = $pkc_encoder->decode($decrypted);                // 去 PKCS#7
if (strlen($result) < 16) return "";
$content    = substr($result, 16, strlen($result));        // 跳过 16 字节 random
$len_list   = unpack("N", substr($content, 0, 4));         // 网络字节序取 msg_len
$xml_len    = $len_list[1];
$xml_content= substr($content, 4, $xml_len);               // 真正的明文
$from_appid = substr($content, $xml_len + 4);              // 尾部 appid
if ($from_appid != $appid) return ErrorCode::$ValidateAppidError;   // -40005
return array(0, $xml_content);
```

官方 PHP `Prpcrypt::encrypt`：

```php
$random = $this->getRandomStr();                          // 16 字符
$text = $random . pack("N", strlen($text)) . $text . $appid;
$iv = substr($this->key, 0, 16);
$text = $pkc_encoder->encode($text);
$encrypted = mcrypt_generic($module, $text);
return array(ErrorCode::$OK, base64_encode($encrypted));
```

> 随机字符串字符集（PHP 实现）：`"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmnopqrstuvwxyz"`，长度 16。

### 6.6 回复包（加密回包）

回包格式（官方原文）：

```xml
<xml>
  <Encrypt><![CDATA[${msg_encrypt}$]]></Encrypt>
  <MsgSignature><![CDATA[${msg_signature}$]]></MsgSignature>
  <TimeStamp>${timestamp}$</TimeStamp>
  <Nonce><![CDATA[${nonce}$]]></Nonce>
</xml>
```

- `Encrypt` 生成方法同上（`FullStr = random(16B)+msg_len(4B)+msg+appid`，`appid` 为**小程序 Appid** 或第三方平台 Appid，按场景）
- `TimeStamp` 由开发者生成（当前时间戳即可）
- `Nonce` **回填 URL 参数中的 nonce**
- `MsgSignature = sha1(sort(Token, TimeStamp(回包中的), Nonce(回包中的), Encrypt(回包中的)))`

官方给出的完整回包示例（消息推送页）：

```xml
<xml>
<Encrypt><![CDATA[hE8R6mGXHkJJjU72KxzKUd1GEkKJaEZq7vRL8XgK3o+00k8JGq6+pZJUIlTSyhsX+bxIBQ72g3GyvDdIZcr6+3HAZbSvPT9t/o11MI7d6WELwqrGd7jMnV0zv3Zc9Nq7]]></Encrypt>
<MsgSignature><![CDATA[03e0812039325c2712ef5f0f980fd14c70d6e307]]></MsgSignature>
<TimeStamp>1713424427</TimeStamp>
<Nonce><![CDATA[415670741]]></Nonce>
</xml>
```

### 6.7 「返回 success 明文」的约定

| 场景 | 必须响应 |
|---|---|
| 授权事件接收 URL（`component_verify_ticket` 推送） | **直接返回字符串 `success`**（明文，不加密） |
| 授权事件接收 URL（authorized/updateauthorized/unauthorized 推送） | **直接返回字符串 `success`** |
| 消息与事件接收 URL（代码审核结果推送 `weapp_audit_*`） | **直接返回字符串 `success`** |
| 消息推送的一般回包 | 官方原文：「首先需确定回包包体的明文内容，具体取决于特定接口文档要求，**如无特定要求，回复空串或者 `success`（无需加密）即可**，其他回包内容需加密处理」 |

### 6.8 EncodingAESKey 轮换（重要运维细节）

官方原文（消息加解密说明 / 技术方案，两页都有）：

> 「出于安全考虑，开放平台网站提供了**修改 EncodingAESKey** 的功能（在 EncodingAESKey 可能泄漏时进行修改），所以建议账号**保存当前的和上一次的 EncodingAESKey**，若当前 EncodingAESKey 解密失败，则尝试用上一次的 EncodingAESKey 解密。**回包时，用哪个 Key 解密成功，则用此 Key 加密对应的回包。**」

### 6.9 视频号小店的 JSON 回包（例外）

官方文档在加解密说明中多处补充：**视频号小店的回包以 JSON 格式返回**，例如：

```json
{ "Encrypt": "...", "MsgSignature": "", "TimeStamp": 1411034505, "Nonce": "" }
```

请求侧对应用 `{"Encrypt":"", "ToUserName":""}`。**第三方平台固定 XML**（见 6.1），该 JSON 分支属于微信小店场景，做第三方平台时可不实现，但解析时留出兼容位更稳。

### 6.10 错误码（官方示例代码 `errorCode.php`）

| 码 | 含义 |
|---|---|
| `0` | OK |
| `-40001` | 签名验证错误 (`ValidateSignatureError`) |
| `-40002` | xml 解析失败 (`ParseXmlError`) |
| `-40003` | sha 加密生成签名失败 (`ComputeSignatureError`) |
| `-40004` | encodingAesKey 非法 (`IllegalAesKey`) |
| `-40005` | appid 校验错误 (`ValidateAppidError`) |
| `-40006` | aes 加密失败 (`EncryptAESError`) |
| `-40007` | aes 解密失败 (`DecryptAESError`) |
| `-40008` | 解密后得到的 buffer 非法 (`IllegalBuffer`) |
| `-40009` | base64 加密失败 (`EncodeBase64Error`) |
| `-40010` | base64 解密失败 (`DecodeBase64Error`) |
| `-40011` | 生成 xml 失败 (`GenReturnXmlError`) |

> 官方常见错误提示还包括：「xml 格式不对：如写成了…（s 小写了且 p 和 > 中间有空格）」。

---

## 7. 错误处理与频率限制

### 7.1 全局限定：第三方平台高频错误码

来源：[公共错误码](https://developers.weixin.qq.com/doc/oplatform/Return_codes/Return_code_descriptions_new.html) + 各接口页。

| errcode | 英文描述 | 中文/处理 |
|---|---|---|
| `-1` | system error | 系统繁忙，稍后重试 |
| `0` | ok | 成功 |
| `40001` | invalid credential, access_token is invalid or not latest | AppSecret 错误，或 access_token 无效/非最新 → **刷新 token 后重试** |
| `40013` | invalid appid | 不合法 AppID，注意大小写与异常字符 |
| `40014` | invalid access_token | 不合法的 access_token，检查有效性/是否过期 |
| `40125` | invalid appsecret | 无效的 appsecret → 检查/重置 AppSecret |
| `41001` | access_token missing | 缺少 access_token 参数 |
| `41002` | appid missing | 缺少 appid 参数 |
| `41004` | appsecret missing | 缺少 secret 参数 |
| `41018` | missing component_appid | 缺少 component_appid（第三方平台接口） |
| `42001` | access_token expired | access_token 超时 → **刷新 token 后重试** |
| `42007` | access_token and refresh_token exception | **用户修改微信密码**，access_token 与 refresh_token 失效，**需要重新授权** |
| `43001` | require GET method | 需要 GET 请求 |
| `43002` | require POST method | 需要 POST 请求 |
| `43003` | require https | 需要 HTTPS 请求 |
| `44002` | empty post data | POST 的数据包为空（很多接口要求 body 传 `{}`） |
| `45002` | content size out of limit | 消息内容超过限制 |
| `45009` | reach max api daily quota limit | 调用超过天级别频率限制 → 可调 `clear_quota` 恢复额度 |
| `45011` | api minute-quota reach limit, must slower, retry next minute | API 调用太频繁，请稍候再试 |
| `45035` | access clientip is not registered, not in ip-white-list | 出口 IP 不在白名单 |
| `40097` | invalid args | 参数错误 |
| `40164` | invalid ip, not in whitelist | 出口 IP 不在白名单 |
| `40170` | args count exceed count limit | 个数超出限制（如 `get_authorizer_list` 的 count） |
| `48001` | api unauthorized | api 功能未授权，确认公众号/小程序已获得该接口权限 |
| `48002` | user block receive message | 粉丝拒收消息 |
| `48004` | api forbidden for irregularities | api 接口被封禁，登录 mp 查看详情 |
| `48006` | forbid to clear quota because of reaching the limit | 清零次数达到上限 |
| `50002` | user limited | 用户受限（账号冻结或注销） |
| `61003` | component is not authorized by this account | 小程序尚未授权给该第三方平台（`/sns/component/jscode2session`） |
| `61004` | access clientip is not registered | **第三方平台出口 IP 未设置** → 把 IP 加入白名单重试 |
| `61005` | component ticket is expired | ticket 过期 |
| `61006` | component ticket is invalid | ticket 无效 |
| `61007` | api is unauthorized to component | **该接口未被授权给第三方平台**（权限集未包含） |
| `61011` | invalid component | 无效的第三方平台 |
| `61014` | must use component token for component api | **用错 token**：应使用 `component_access_token` |
| `61016` | function category of API need be confirmed by component | 接口权限集需确认 |
| `61025` | read-only option | 只读选项 |
| `61039` | 隐私接口检查任务未完成，请稍等一分钟再重试 | |
| `61040` | ext.json 配置的隐私接口无权限 / 代码中含有未配置的隐私接口 | |
| `85064` | template not found | 找不到模板 |
| `85065` | template list is full | **模板库已满（上限 200）** |
| `85019` | no version is under auditing | 没有审核版本 |
| `85020` | status not allowed | 审核状态未满足发布 |
| `85009` | already submit a version under auditing | 已经有正在审核的版本 |
| `85012` | invalid audit id | 无效的审核 id |
| `85023` | item size is not in valid range | 审核列表项目数不在 1-5 以内 |
| `85044` | package exceed max limit | **代码包超过大小限制** |
| `85045` ~ `85048` | ext_json 路径不存在 / tabBar 缺 path / pages 为空 / ext_json 解析失败 | |
| `85051` | data too large | version_desc 或 preview_info 超限 |
| `85077` | 小程序类目信息失效 | |
| `85079` ~ `85082` | 分阶段发布相关（无线上版本 / 提审未通过 / 无效比例 / 比例过低） | |
| `85085` | submit audit reach limit | **小程序提审数量已达本月上限** |
| `85086` | must commit before submit audit | 提审前需先上传代码 |
| `85087` | 已使用 navigateToMiniProgram，请声明跳转 appid 列表 | |
| `85092` ~ `85094` | preview_info 格式错误 / 个数超限 / 需提供审核机制说明 | |
| `85310` ~ `85312` | requiredPrivateInfos 格式/名称错、互斥、无权限 | |
| `86000` | should be called only from third party | 不是由第三方代小程序进行调用 |
| `86001` | component experience version not exists | 不存在第三方的已经提交的代码 |
| `86002` | miniprogram have not completed init procedure | 小程序还未设置昵称、头像、简介 |
| `86007` | 小程序禁止提交 | |
| `86009` / `86010` | 服务商新增/迭代小程序代码提审能力被限制 | |
| `87006` | this is game miniprogram submitaudit is forbidden | 小游戏不能提交 |
| `87012` | forbid revert this version release | 该版本不能回退 |
| `87013` | no quota to undo code | 撤回次数达上限（每天 5 次、每月 10 次） |
| `89014` | support version error | 基础库版本输入错误 |
| `89401` ~ `89405` | 加急审核：系统不稳定 / 不在待审核队列 / 不支持加急种类 / 已加速成功 / 本月加急额度已用完 | |
| `9400001` | 开发小程序已开通直播权限，不支持发布版本 | |
| `9402202` | concurrent limit | 请勿频繁提交，待上一次操作完成后再提交 |
| `9402203` | 标准模板 ext_json 错误 | |

> **`40001` / `42001` 的标准处理**：清空本地缓存的 `component_access_token`（或 `authorizer_access_token`），重新获取后**重试一次**。
> **`40014`** 在多份 2.0 老文档中被写作"invalid access_token"，与 `40001` 语义接近，建议两者都触发"强制刷新 token 并重试一次"。
> **`42007`** 无法自动恢复：用户改密码后 access_token/refresh_token 双失效，**必须让商家重新授权**。

### 7.2 API 调用频率限制

**（a）通用机制**（[接口调用额度说明](https://developers.weixin.qq.com/doc/service/guide/dev/api/limit.html)）

> 「服务号调用接口并不是无限制的……当超过一定限制时，调用对应接口会收到如下错误返回码：
> `{"errcode":45009,"errmsg":"api freq out of limit"}`」

- 「**第三方帮助服务号调用时，实际上是在消耗服务号自身的 quota。**」
- 「每个账号**每月共 10 次清零操作机会**，清零生效一次即用掉一次机会（10 次包括了平台上的清零和调用接口 API 的清零）。」
- 认证账号可对实时调用量清零；错误码 `48006` 表示清零次数达到上限。
- 官方在该页给出的**服务号**每日限额表（节选，第三方平台自身额度表**官方未在此页列出**）：
  | 接口 | 每日限额 |
  |---|---|
  | 获取 access_token | **2000** |
  | 获取网页授权 access_token | 无 |
  | 刷新网页授权 access_token | 无 |
  | 自定义菜单创建 | 1000 |
  | 发送客服消息 | 500000 |

- 测试号限额另有一张表（获取 access_token 每日 200）。

**（b）第三方平台：component_access_token 每日获取次数**

- 官方 `api_component_token` 页面只写：「令牌的获取是有限制的，每个令牌的有效期为 2 小时……」，
  **没有给出具体次数**；超限返回 `45009`。
- 网上流传的固定数字**未在官方文档中出现** → 标注：**文档未明确**。
- 实现建议：**严格缓存 token**，只在 `expires_at - 600s` 之后刷新一次（单实例 + 分布式锁），避免多实例各刷一次把额度刷爆。

**（c）小程序代码上传 / 提审频次**

- `/wxa/commit` 文档**未给出上传频次数字**，只给出 `9402202 concurrent limit`（"请勿频繁提交，待上一次操作完成后再提交"）与 `85044`（包大小）。
- `/wxa/submit_audit` 的额度机制：`85085`（本月提审数量已达上限）→ 官方指引见
  [第三方小程序提审问题](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/troubleshooting/submit_quota.html)：「服务商通过 API 代商家提交小程序代码审核，有一定的 quota……如果 quota 不足继续调用提交审核接口，会出现 `85085` 报错。」
- 服务商可用 `GET /wxa/queryquota` 查询：`rest`/`limit`（提审额度）与 `speedup_rest`/`speedup_limit`（加急额度），**旗下所有小程序共用**。
- `/wxa/undocodeaudit`：**每天 ≤ 5 次，每月 ≤ 10 次**（`87013`）。
- `/wxa/speedupaudit`：`89405` 本月加急额度已用完。
- 具体"每天可上传几次代码"的数字：**文档未明确**。

**（d）其他已知额度**

- 全局注册类接口：`91030 reach wxid freq limit` —— 官方社区回复：「**单个微信号在单个第三方平台账号发起的次数 5 次/天**（调用接口成功/失败都算）」。（微信社区，非文档页 — 标注为社区口径）

**（e）查询/清零额度接口**

| 接口 | Method + URL | 鉴权 | Body/返回 |
|---|---|---|---|
| 查询 API 调用额度 | `POST https://api.weixin.qq.com/cgi-bin/openapi/quota/get?access_token=ACCESS_TOKEN` | 第三方平台接口 → `component_access_token`；第三方平台接口但用于公众号/小程序 → `authorizer_access_token` | body `{"cgi_path":"/wxa/gettemplatedraftlist"}`（**不要带 `https://api.weixin.qq.com` 前缀，也要保留开头的 `/`**，否则 `76003`）；返回 `quota{daily_limit,used,remain}`、`rate_limit{call_count,refresh_second}`、`component_rate_limit{call_count,refresh_second}` |
| 重置 API 调用次数 | `POST https://api.weixin.qq.com/cgi-bin/clear_quota?access_token=ACCESS_TOKEN` | 平台自身用 `component_access_token`；代调用用 `authorizer_access_token` | 清空每日调用次数；`48006` |
| 使用 AppSecret 重置第三方平台 API 调用次数 | `POST https://api.weixin.qq.com/cgi-bin/component/clear_quota/v2` | 用 appid+appsecret（见页面） | `48006` |

> `/xxx/sns/xxx` 这类接口不支持 quota 查询（`76022`）。
> 「如果接口文档中有单独的说明接口的特殊的 quota 数量以及逻辑，则以每个接口的接口文档的描述为准。」

---

## 8. 官方 Demo 代码

### 8.1 下载地址（实测可下载）

| 来源页面 | 下载链接 | 大小 | 说明 |
|---|---|---|---|
| [消息加解密说明（第三方平台 2.0）](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/Message_encryption_and_decryption.html) 中的「点击下载」 | `https://res.wx.qq.com/op_res/-serEQ6xSDVIjfoOHcX78T1JAYX-pM_fghzfiNYoD8uHVd3fOeC0PC_pvlg4-kmP` | **284,876 B (zip)** | 官方 WXBizMsgCrypt 示例代码（**5 种语言，50 个文件**），**已实测 HTTP 200、可解压** |
| [消息加解密技术介绍](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/Technical_Plan.html) 中的「示例下载」 | `https://wximg.gtimg.com/shake_tv/mpwiki/cryptoDemo.zip` | 4,663,403 B (zip) | 同名代码的**更大版本（387 个文件，含 C++ 的 openssl 头文件/lib 与 .svn 元数据）**，已实测 HTTP 200、可解压 |

> ### ⚠️ 关于 `https://developers.weixin.qq.com/doc/oplatform/Downloads/demo.html`
> 该 URL 目前**返回"页面不存在或已失效"**（HTTP 200 但内容是 404 提示）。抓取时该页不可用。
> **可用的官方 demo 下载链接是上表两个**（都是从现行文档页里提取的真实 href）。
> → 标注：**任务书给出的 Demo 下载页当前无法访问**。

### 8.2 语言列表与代码结构

官方原文：「微信公众平台提供了 **c++, php, java, python, c#** 5 种语言的示例代码（点击下载，请运行示例代码前先阅读对应的 readme 文件），**每种语言的类名和接口名均一致**。」

`op_res_demo.zip` 解压后的实际结构（`SampleCode/SampleCode/`）：

```
SampleCode/SampleCode/
├── php/
│   ├── wxBizMsgCrypt.php     (4,111 B)   ← WXBizMsgCrypt 主类
│   ├── pkcs7Encoder.php      (4,262 B)   ← PKCS7Encoder + Prpcrypt（AES 加解密核心）
│   ├── sha1.php              (726 B)     ← SHA1 签名类
│   ├── xmlparse.php          (1,387 B)   ← XML 提取 Encrypt/ToUserName、生成回包 XML
│   ├── errorCode.php         (1,089 B)   ← -40001 ~ -40011
│   ├── demo.php              (1,614 B)   ← 调用样例
│   └── ReadMe.txt            (411 B)
├── Java/
│   ├── src/com/qq/weixin/mp/aes/
│   │   ├── WXBizMsgCrypt.java (10,084 B)
│   │   ├── PKCS7Encoder.java  (1,684 B)
│   │   ├── SHA1.java          (1,590 B)
│   │   ├── XMLParse.java      (2,657 B)
│   │   ├── ByteGroup.java     (521 B)
│   │   ├── AesException.java  (1,688 B)
│   │   └── WXBizMsgCryptTest.java (6,059 B)
│   ├── src/demo/Program.java  (2,766 B)
│   ├── commons-codec-1.9.jar / dist/aes-jre1.6.jar
│   └── readme.txt
├── C++/
│   ├── src/WXBizMsgCrypt.cpp/.h, Sample.cpp, Makefile
│   └── lib/include32/…
├── Python/
│   ├── WXBizMsgCrypt.py (9,400 B)
│   ├── Sample.py        (2,372 B)
│   ├── ierror.py        (795 B)
│   └── __init__.py
└── C#/
    ├── WXBizMsgCrypt.cs (8,769 B)
    ├── Cryptography.cs  (8,581 B)
    ├── Sample.cs        (4,817 B)
    └── Readme.txt
```

> **注意：官方没有提供 Go 版本**，需要自行按上述算法实现。

### 8.3 类结构与接口（用于 Go 重新实现的对照表）

官方 C++ 注释给出的类接口（三种语言类名/接口名一致）：

```
构造函数：WXBizMsgCrypt(sToken, sEncodingAESKey, sAppid)

int DecryptMsg(sMsgSignature, sTimeStamp, sNonce, sPostData, &sMsg);
   // 检验消息真实性并获取解密后明文
   // sMsgSignature ← URL 参数 msg_signature
   // sTimeStamp    ← URL 参数 timestamp
   // sNonce        ← URL 参数 nonce
   // sPostData     ← POST 原始数据
   // return 成功 0，失败返回错误码

int EncryptMsg(sReplyMsg, sTimeStamp, sNonce, &sEncryptMsg);
   // 将待回复消息加密打包成含 msg_signature/timestamp/nonce/encrypt 的 XML
```

**Go 侧建议的模块划分（1:1 对应官方 PHP 文件）**

| 官方 PHP 文件 | Go 对应 | 职责 |
|---|---|---|
| `wxBizMsgCrypt.php` | `wxcrypt/crypt.go` | 构造函数 + `DecryptMsg` + `EncryptMsg` |
| `sha1.php` | `wxcrypt/signature.go` | 四元组字典序排序 + sha1 |
| `xmlparse.php` | `wxcrypt/xml.go` | 解析 `Encrypt`/`ToUserName`；生成回包 XML |
| `pkcs7Encoder.php` | `wxcrypt/pkcs7.go` | PKCS#7（**block size = 32**） |
| （同上 Prpcrypt 类） | `wxcrypt/aes.go` | `aes.NewCipher(key)` + `cipher.NewCBCDecrypter/Encrypter`，**IV = key[:16]** |

**关键实现坑位清单（Go）**

1. `aes.NewCipher` 要求 key 长度 16/24/32；这里固定 **32**。若 `EncodingAESKey` 解码后不是 32 字节 → 返回 `-40004`。
2. **PKCS#7 用 block size 32，不是 16**（Go 标准库里没有，必须自己写）。
3. 必须**手动补齐/截断 base64 输入**：`base64.StdEncoding` 对无 padding 的输入报错。微信推送的 base64 是带 padding 的；但 EncodingAESKey 需要 `+ "="`。
4. 签名比较务必用**常量时间比较**（`subtle.ConstantTimeCompare`）或至少 `hmac.Equal`。
5. `msg_len` 是 **big-endian**：Go 用 `binary.BigEndian.Uint32(b[16:20])`。
6. `appid` 从 `b[20+msgLen:]` 取，需与本地第三方平台 appid **严格字符串比较**。
7. XML 解析：`encoding/xml` 对 `<![CDATA[...]]>` 支持良好，但要注意 `<Encrypt>` 内容可能含 `+` `/` `=`，**不要做 HTML unescape**。
8. XML 外部实体（XXE）：官方 PHP 里调用了 `libxml_disable_entity_loader(true)` 防 XXE，Go 的 `encoding/xml` 默认不解析外部实体，安全。
9. 回包 XML 结构与官方 `xmlparse.php::generate` 完全一致（`Encrypt`/`MsgSignature`/`TimeStamp`/`Nonce`，前二者与 `Nonce` 包 CDATA，`TimeStamp` 不包）。

---

## 附录 A：未能确认 / 文档未明确项

| # | 事项 | 状态 |
|---|---|---|
| 1 | `https://developers.weixin.qq.com/doc/oplatform/Downloads/demo.html`（第三方平台 Demo 下载页） | **页面返回"页面不存在或已失效"**；已改用文档页内真实下载链接（见 8.1） |
| 2 | `https://mp.weixin.qq.com/mp/authorize?...` 移动端授权链接 | **官方文档未收录**；官方移动端授权为 H5 版 `open.weixin.qq.com/wxaopen/safe/bindcomponent` |
| 3 | `/wxa/submit_audit/v2`、`auto_audit`、`auto_reject` | **官方文档未收录**（中/英、新/旧路径均只有 `/wxa/submit_audit`，且无自动提审参数） |
| 4 | 预授权码「使用次数」 | **文档未明确**（只说明有效期，且 1800s 与示例 600 冲突） |
| 5 | `api_authorizer_token` 刷新后 `authorizer_refresh_token` 是否一定变化 | **文档未明确**；建议每次覆盖保存 |
| 6 | `get/set_authorizer_option` 的 `authorizer_appid` / `component_appid` 位置 | **新版文档请求体表缺失这两个字段**（仅英文版另有"use authorizer_access_token"的说法，与中文版"可使用 component_access_token"矛盾）→ 需实测 |
| 7 | 小程序专属 `option_name` | **文档未列出**（只有 location_report / voice_recognize / customer_service） |
| 8 | `component_access_token` 每日获取次数上限的具体数字 | **文档未明确**（只写"令牌的获取是有限制的"，超限 `45009`） |
| 9 | `/wxa/commit` 每日上传频次上限 | **文档未明确**（只有 `9402202 concurrent limit`） |
| 10 | `ext_json` 中是否支持 `debug` 字段 | **文档未明确**（官方示例出现的是 `plugin`，未见 `debug`） |
| 11 | `get_authorizer_info` 的 `MiniProgramInfo.network` 是否有 `BizDomain` | 字段表未列，示例中出现 → **文档未明确** |
| 12 | 重置 AppSecret 后对已签发 token 的即时影响、冷却期 | **文档未明确** |
| 13 | `get_authorizer_list` 返回的 `refresh_token` 是否可能为空 | 新文档未说明；FAQ 提到的"返回空"场景未指明是哪个接口 → **文档未明确** |
| 14 | `nickname`（任务书用词） | 官方字段名是 **`nick_name`**，不存在 `nickname` 字段 |
| 15 | `wxa_audit_status` 事件名（任务书用词） | 官方事件名是 **`weapp_audit_success` / `weapp_audit_fail` / `weapp_audit_delay`** |
| 16 | `/wxa/get_release_status`（任务书用词） | 官方无此接口；等价能力为 `/wxa/getversioninfo` |
| 17 | `api_get_authorizer_option` / `api_set_authorizer_option`（任务书用词） | 官方 URL 路径**不带 `api_` 前缀**：`/cgi-bin/component/get_authorizer_option`、`/set_authorizer_option`（`api_*` 只是接口英文名的文档命名习惯） |
| 18 | 小程序代码包 2MB | 单包/主包 **2M** 确认；总量为 **代开发 ≤ 20M**（普通小程序 30M），来源为[分包加载](https://developers.weixin.qq.com/miniprogram/dev/framework/subpackages.html)，非 `/wxa/commit` 页 |

---

## 附录 B：引用链接

**令牌 / 授权**
- [获取令牌 getComponentAccessToken](https://developers.weixin.qq.com/doc/oplatform/openApi/ticket-token/api_getcomponentaccesstoken.html)
- [令牌（2.0 旧路径）](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/ThirdParty/token/component_access_token.html)
- [验证票据 component_verify_ticket](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/component_verify_ticket.html)
- [authorizer_access_token 生成说明](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/creat_token.html)
- [授权变更通知推送](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/authorize_event.html)
- [启动票据推送服务 startPushTicket](https://developers.weixin.qq.com/doc/oplatform/openApi/ticket-token/api_startpushticket.html)
- [获取预授权码 getPreAuthCode](https://developers.weixin.qq.com/doc/oplatform/openApi/ticket-token/api_getpreauthcode.html)
- [预授权码（2.0 旧路径）](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/ThirdParty/token/pre_auth_code.html)
- [获取刷新令牌 getAuthorizerRefreshToken（api_query_auth）](https://developers.weixin.qq.com/doc/oplatform/openApi/ticket-token/api_getauthorizerrefreshtoken.html)
- [获取授权账号调用令牌 getAuthorizerAccessToken（api_authorizer_token）](https://developers.weixin.qq.com/doc/oplatform/openApi/ticket-token/api_getauthorizeraccesstoken.html)
- [获取/刷新接口调用令牌（2.0 旧路径）](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/ThirdParty/token/api_authorizer_token.html)
- [授权流程技术说明](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/Authorization_Process_Technical_Description.html)
- [技术说明（英文版，含 H5 旧版链接）](https://developers.weixin.qq.com/doc/oplatform/en/Third-party_Platforms/Authorization_Process_Technical_Description.html)
- [如何代商家调用接口](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/getting_started/how_to_call_api.html)
- [如何查看和重置 AppSecret](https://developers.weixin.qq.com/doc/oplatform/developers/dev/appid.html)
- [如何成为服务商](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/getting_started/how_to_be.html)

**授权方信息**
- [获取授权账号详情 getAuthorizerInfo](https://developers.weixin.qq.com/doc/oplatform/openApi/authorization-management/api_getauthorizerinfo.html)
- [拉取已授权的账号信息 getAuthorizerList](https://developers.weixin.qq.com/doc/oplatform/openApi/authorization-management/api_getauthorizerlist.html)
- [获取授权方选项信息 getAuthorizerOptionInfo](https://developers.weixin.qq.com/doc/oplatform/openApi/authorization-management/api_getauthorizeroptioninfo.html)
- [设置授权方选项信息 setAuthorizerOptionInfo](https://developers.weixin.qq.com/doc/oplatform/openApi/authorization-management/api_setauthorizeroptioninfo.html)
- [小程序权限集说明](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/product/miniprogram_authority.html)

**模板库 / 代开发**
- [获取草稿箱列表 getTemplatedRaftList](https://developers.weixin.qq.com/doc/oplatform/openApi/thirdparty-management/template-management/api_gettemplatedraftlist.html)
- [将草稿添加到模板库 addToTemplate](https://developers.weixin.qq.com/doc/oplatform/openApi/thirdparty-management/template-management/api_addtotemplate.html)
- [获取模板列表 getTemplateList](https://developers.weixin.qq.com/doc/oplatform/openApi/thirdparty-management/template-management/api_gettemplatelist.html)
- [删除代码模板 deleteTemplate](https://developers.weixin.qq.com/doc/oplatform/openApi/thirdparty-management/template-management/api_deletetemplate.html)
- [小程序模板库管理（200 个上限）](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/operation/thirdparty/template.html)
- [服务商代开发小程序](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/product/how_to_dev.html)
- [第三方平台开发配置](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/operation/thirdparty/config.html)

**代码管理（代商家）**
- [上传代码并生成体验版 commit](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_commit.html)
- [上传小程序代码并生成体验版（2.0 旧路径）](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/code/commit.html)
- [获取已上传的代码页面列表 getCodePage](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getcodepage.html)
- [获取体验版二维码 getTrialQRCode](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_gettrialqrcode.html)
- [提交代码审核 submitAudit](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_submitaudit.html)
- [提交审核（2.0 旧路径）](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/code/submit_audit.html)
- [查询审核单状态 getAuditStatus](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getauditstatus.html)
- [查询最新一次审核单状态 getLatestAuditStatus](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getlatestauditstatus.html)
- [撤回代码审核 undoAudit](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_undoaudit.html)
- [发布已通过审核的小程序 release](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_release.html)
- [小程序版本回退 revertCodeRelease](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_revertcoderelease.html)
- [查询小程序版本信息 getVersionInfo](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getversioninfo.html)
- [加急代码审核 speedupCodeAudit](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_speedupcodeaudit.html)
- [加急审核申请（2.0 旧路径）](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/code/speedup_audit.html)
- [查询服务商审核额度 setCodeAuditQuota](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_setcodeauditquota.html)
- [上传提审素材 uploadMediaToCodeAudit](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_uploadmediatocodeaudit.html)
- [获取隐私接口检测结果 getCodePrivacyInfo](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getcodeprivacyinfo.html)
- [分阶段发布 grayRelease](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_grayrelease.html)
- [获取分阶段发布详情 getGrayReleasePlan](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getgrayreleaseplan.html)
- [取消分阶段发布 revertGrayRelease](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_revertgrayrelease.html)
- [设置小程序服务状态 setVisitStatus](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_setvisitstatus.html)
- [查询小程序服务状态 getVisitStatus](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getvisitstatus.html)
- [查询各版本用户占比 getSupportVersion](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getsupportversion.html)
- [设置最低基础库版本 setSupportVersion](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_setsupportversion.html)
- [小程序登录（服务商）thirdpartyCode2Session](https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/login/api_thirdpartycode2session.html)
- [分包加载（2M / 20M / 30M）](https://developers.weixin.qq.com/miniprogram/dev/framework/subpackages.html)
- [使用分包](https://developers.weixin.qq.com/miniprogram/dev/framework/subpackages/basic.html)

**加解密**
- [消息加解密说明](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/Message_encryption_and_decryption.html)
- [消息加解密技术介绍（加密解密技术方案）](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/Technical_Plan.html)
- [消息推送（含完整数值示例）](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/message_push.html)
- 示例代码下载：`https://res.wx.qq.com/op_res/-serEQ6xSDVIjfoOHcX78T1JAYX-pM_fghzfiNYoD8uHVd3fOeC0PC_pvlg4-kmP`
- 示例代码（含 C++ 依赖）下载：`https://wximg.gtimg.com/shake_tv/mpwiki/cryptoDemo.zip`

**错误码 / 频率**
- [公共错误码](https://developers.weixin.qq.com/doc/oplatform/Return_codes/Return_code_descriptions_new.html)
- [接口调用额度说明](https://developers.weixin.qq.com/doc/service/guide/dev/api/limit.html)
- [查询 API 调用额度 getApiQuota](https://developers.weixin.qq.com/doc/oplatform/openApi/openapi/api_getapiquota.html)
- [重置 API 调用次数 clearQuota](https://developers.weixin.qq.com/doc/oplatform/openApi/openapi/api_clearquota.html)
- [使用 AppSecret 重置第三方平台 API 调用次数](https://developers.weixin.qq.com/doc/oplatform/openApi/openapi/api_componentclearquota_v2.html)
- [第三方小程序提审问题](https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/troubleshooting/submit_quota.html)

---

## 附录 C：Go 实现落地检查清单（提炼）

1. **Token 管理器**：内存/Redis 缓存 `component_access_token`（TTL 7200s，提前 600s 刷新）+ 每个 `authorizer_appid` 的 `authorizer_access_token`（同上）+ `authorizer_refresh_token` 持久化。
2. **ticket 接收**：「授权事件接收 URL」实现 `GET`（验证 `signature`，返回 `echostr`）与 `POST`（**用 `msg_signature` 验签，不要用 `signature`**）→ 解密 → 若 `InfoType==component_verify_ticket` 则持久化 ticket（含 12h 有效期），**统一返回 `success`**。
3. **授权事件处理**：`authorized`/`updateauthorized` → 用 `AuthorizationCode` 调 `api_query_auth` 落库；`unauthorized` → 标记失效并清理 token 缓存。
4. **加解密库**：按第 6 节实现，重点：block size 32 的 PKCS#7、`IV=AESKey[:16]`、`msg_len` big-endian、保存当前+上一次 EncodingAESKey。
5. **代调用客户端**：`/wxa/*` 一律用 `authorizer_access_token`；`/cgi-bin/component/*` 与模板库接口一律用 `component_access_token`；收到 `40001/40014/42001` → 刷新后重试一次；收到 `42007` → 标记需重新授权。
6. **强制 `{}` body**：`/wxa/release`、`/wxa/getversioninfo`、`/wxa/getvisitstatus`、`/cgi-bin/wxopen/getweappsupportversion` 必须 POST 空 JSON，否则 `44002`。
7. **`/wxa/get_qrcode`** 返回二进制流，需按 `Content-Type` 分流。
8. **版本上限**：模板库 ≤ 200；回退版本最多保留最近 5 个；撤回审核每天 5 次/每月 10 次。
9. **提交审核前**：先 `getCodePrivacyInfo` 确认检测任务完成，再 `submit_audit`；不要把 `commit` 与 `submit_audit` 放在同一个重试策略里。
