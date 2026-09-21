# internal/wxapi 实现的接口矩阵与文档冲突备注

本文件是 `internal/wxapi` 的实现备忘（代码注释是第一手说明，此处汇总便于上层核对）。
所有结论都按「先看 `docs/wx-docs/*.md` 单页文档，其次 `docs/reference/wx-open-platform-api.md`」
的顺序核对；仓库内没有单页文档的接口（类目 / 隐私 / 域名组）用官方 openApi 单页在线文档核对，出处见各行。

## 1. 文档要求 GET 的接口（实现为 GET，不带 body）

| 路径 | 令牌 | 权限集 | 依据 |
|---|---|---|---|
| `/wxa/gettemplatedraftlist` | component_access_token | —（第三方平台自身） | `tpl_gettemplatedraftlist.md` |
| `/wxa/gettemplatelist`（可选 `template_type` URL 参数） | component_access_token | — | `tpl_gettemplatelist.md` |
| `/wxa/get_page` | authorizer_access_token | 18 | `code_getcodepage.md` |
| `/wxa/get_qrcode`（`path` 需 urlencode） | authorizer_access_token | 18、86 | `code_gettrialqrcode.md` |
| `/wxa/get_latest_auditstatus` | authorizer_access_token | 18 | `code_getlatestauditstatus.md` |
| `/wxa/undocodeaudit` | authorizer_access_token | 18 | `code_undoaudit.md` |
| `/wxa/revertcoderelease`（`app_version` / `action=get_history_version` 均为 **URL 参数**） | authorizer_access_token | 18 | `code_revertcoderelease.md` |
| `/wxa/queryquota` | authorizer_access_token | 18 | `code_setcodeauditquota.md` |
| `/wxa/getgrayreleaseplan` | authorizer_access_token | 18 | `code_getgrayreleaseplan.md` |
| `/wxa/revertgrayrelease` | authorizer_access_token | 18 | `code_revertgrayrelease.md` |
| `/wxa/security/get_code_privacy_info` | authorizer_access_token | 18 | `code_getcodeprivacyinfo.md` |
| `/wxa/get_category` | authorizer_access_token | 18 | 官方 online：`api_getallcategoryname.html` |
| `/cgi-bin/wxopen/getcategory` | authorizer_access_token | **30** | 官方 online：`api_getsettingcategories.html` |

## 2. 文档要求 POST 且**必须发空 JSON `{}`** 的接口（否则 `44002 empty post data`）

| 路径 | 令牌 | 依据 |
|---|---|---|
| `/wxa/release` | authorizer_access_token | `code_release.md` 注意事项原文「post 的 data 为空，不等于不需要传 data」 |
| `/wxa/getversioninfo` | authorizer_access_token | `code_getversioninfo.md` 错误码表 `44002` |
| `/wxa/getvisitstatus` | authorizer_access_token | `code_getvisitstatus.md` 错误码表 `44002` |
| `/cgi-bin/wxopen/getweappsupportversion` | authorizer_access_token | `code_getsupportversion.md` + reference §C.6 |
| `/wxa/get_effective_domain` | authorizer_access_token | 官方 online 单页为 **POST**（请求体无字段）+ `server/internal/model/errcodes.go` 的 `44002` 提示点名该接口 |
| `/wxa/get_effective_webviewdomain` | authorizer_access_token | 官方 online 示例请求体即 `{}` |
| `/wxa/get_webviewdomain_confirmfile` | authorizer_access_token | 官方 online 示例请求体即 `{}` |
| `/cgi-bin/component/get_domain_confirmfile` | component_access_token | 官方 online「请求体：无」；统一发 `{}` 更安全 |
| `/cgi-bin/component/getprivacysetting`（`privacyVer == nil` 时） | authorizer_access_token | 官方 online：`privacy_ver` 默认 2，但 body 不能为空 |

## 3. 其余接口（POST + 业务字段 body）

- `/cgi-bin/component/api_component_token`（无令牌）、`/cgi-bin/component/api_start_push_ticket`（无令牌）
- `/cgi-bin/component/api_create_preauthcode`、`api_query_auth`、`api_authorizer_token`（component_access_token）
- `/cgi-bin/component/api_get_authorizer_list`、`api_get_authorizer_info`、`get_authorizer_option`、`set_authorizer_option`（component_access_token）
- `/wxa/addtotemplate`、`/wxa/deletetemplate`（component_access_token）
- `/wxa/commit`、`/wxa/submit_audit`、`/wxa/get_auditstatus`、`/wxa/speedupaudit`、`/wxa/grayrelease`、
  `/wxa/change_visitstatus`、`/cgi-bin/wxopen/setweappsupportversion`、
  `/wxa/modify_domain`、`/wxa/setwebviewdomain`、`/wxa/modify_domain_directly`、`/wxa/setwebviewdomain_directly`
  （authorizer_access_token，权限集 18）
- `/wxa/uploadmedia`（multipart/form-data，字段名固定 `media`；图片 ≤2M、视频 ≤10MB）
- `/cgi-bin/component/modify_wxa_server_domain`、`/cgi-bin/component/modify_wxa_jump_domain`（component_access_token）
- `/cgi-bin/component/getprivacysetting`（`privacyVer != nil` 时 body 为 `{"privacy_ver": N}`）

## 4. 与 docs/reference/wx-open-platform-api.md / 任务书措辞的冲突与取值

1. **`/wxa/get_effective_domain` 的方法**：任务书要求清单里把它写成 GET；官方单页文档写 **POST**，
   且 `server/internal/model/errcodes.go:42` 明确把 `get_effective_domain` 与 `release/getversioninfo/getvisitstatus`
   并列，提示「该接口必须传空 JSON {}」。→ **本实现取 POST + `{}`**（两处内证 vs 任务书一处的措辞）。
   若上层坚持 GET，只需把该方法里的 `postEmpty` 换成 `getJSON` 并去掉 `{}` 断言。
2. **第三方平台业务域名**：新版官方 openApi 页 = `POST /cgi-bin/component/modify_wxa_jump_domain`，字段 `wxa_jump_h5_domain`；
   旧版 2.0 页 = `/cgi-bin/component/setwebviewdomain`，字段 `webviewdomain`。→ **以新版为准**（代码注释已标）。
3. **第三方平台校验文件**：新版 = `POST /cgi-bin/component/get_domain_confirmfile`（不是 `get_webviewdomain_confirmfile`）；
   `get_webviewdomain_confirmfile` 是小程序（代商家）侧接口。→ 两个接口分别按各自新版路径实现。
4. **`get/set_authorizer_option` 的鉴权字段**：新版文档请求体表只列了 `option_name`（reference 附录 A#6 标注「需实测」），
   但实际必须带 `component_appid` + `authorizer_appid`。→ 三个字段都发，冲突写在方法注释里。
5. **`func_info` 结构**：官方为 `[{"funcscope_category":{id,type,name,desc}}]`，任务书 `FuncScope` 是扁平结构。
   → 给 `FuncScope` 实现 `UnmarshalJSON`，嵌套与扁平两种都兼容（公开结构体保持逐字不变）。
6. **`submit_audit` 的 `auditid`**：字段表写 number，示例/真实返回可能是 string。→ 自定义 `UnmarshalJSON` 兼容两者；
   `get_auditstatus` / `get_latest_auditstatus` 的 `auditid` 同样兼容。
7. **`screenshot` / `ScreenShot`**：`code_getauditstatus.md` 字段表用小写，`code_getlatestauditstatus.md` 与推送示例用大写。
   → `AuditStatusResponse` 两个字段都解析。
8. **`version_list`**：`code_revertcoderelease.md` 把类型写成 object，实际是数组。→ 数组 / 单个对象 / 缺失三种都兼容。
9. **`gettemplatelist` 的 `template_type`**：文档写在「请求体」表里，但接口是 GET（无 body）。→ 实现为 URL 查询参数。
10. **`commit` 的 `template_id`**：字段表 number，官方示例是字符串 `"0"`/`"95"`。→ 按任务书结构体发 number（微信两者都接受）。
11. **预授权码有效期**：`token_preauthcode.md` 正文写 1800 秒，返回示例 `expires_in: 600`。→ 以响应字段为准，不写死。
12. **`Options.MaxQPS`**：任务书同时写「默认 8」与「`<=0` 表示不限」，对 0 的含义互斥。
    → `0` 用默认 8（Go 零值无法与「未设置」区分），**负数**表示不限流；已写在 `New` 的注释里。
13. **`ModifyDomainResponse`**：官方 `modify_domain` 还返回 `invalid_wsrequestdomain` /
    `invalid_downloaddomain` / `invalid_udpdomain` / `invalid_tcpdomain`，任务书结构体未列 → 未新增字段（按任务书逐字），
    仅解析已列字段；为不丢失 `ModifyJumpDomainDirectly` 的返回值，补充了一个 `WebviewDomain` 字段（已列字段原样保留）。
14. **`GetEffectiveServerDomain` / `GetEffectiveJumpDomain` 的返回类型**：官方是按来源分组的嵌套结构，
    任务书签名固定为 `map[string][]string` → 前者打平为 `"mp_domain.requestdomain"` 形式（后者本身就是顶层数组）。
15. **不存在的接口名（reference 附录 A 结论，勿按任务书旧名实现）**：`/wxa/get_release_status` → 用 `/wxa/getversioninfo`；
    `nickname` → 官方字段是 `nick_name`；`wxa_audit_status` 事件 → 官方是 `weapp_audit_success/fail/delay`。
