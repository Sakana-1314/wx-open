====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/creat_token.html
====================================================================================================
# # authorizer_access_token 生成说明
 在完成了商家授权给第三方平台之后，第三方平台则可以获得 authorizer_access_token 进行调用相关api，本章节则主要介绍生成 authorizer_access_token 的流程。

| 步骤 | 说明 |
|---|---|
| 1、配置授权事件URL，用于接收component_verify_ticket | 出于安全考虑，在第三方平台创建审核通过后，微信服务器 每隔 10 分钟会向第三方的消息接收地址推送一次 component_verify_ticket，用于获取第三方平台接口调用凭据。component_verify_ticket有效期为 12h |
| 2、获得component_verify_ticket后，按照获取 component_access_token 接口文档调用接口获取 component_access_token | component_access_token有效期 2h，当遭遇异常没有及时收到component_verify_ticket时，建议以上一次可用的component_verify_ticket继续生成component_access_token。避免出现因为 component_verify_ticket 接收失败而无法更新 component_access_token 的情况。 |
| 3、获得component_access_token后，按照获取预授权码 pre_auth_code接口文档调接口获取 pre_auth_code | 用于生成扫码授权二维码或者链接需要的 pre_auth_code |
| 4、获得pre_auth_code后，按照授权技术流程说明文档 ，引导用户授权后获取 authorization_code | authorization_code 有过期时间，该过期时间在授权后回调 URI 进行返回；也会通过推送授权变更通知的方式将authorization_code推送给第三方平台 |
| 5、获得 authorization_code后，按照getAuthorizerRefreshToken 接口文档调接口获取 authorizer_refresh_token | 通过授权码和自己的接口调用凭据（component_access_token），换取授权账号的接口调用凭据（authorizer_access_token 和用于前者快过期时用来刷新它的 authorizer_refresh_token）和授权信息（授权了哪些权限等信息） |
| 6、获得 authorizer_refresh_token 后，按照getAuthorizerAccessToken 接口文档调接口获取 authorizer_access_token | 可通过 authorizer_refresh_token 获取授权账号的接口的调用令牌 |
| 7、按照接口文档，代替公众号或小程序调用接口 | 在完成授权后，第三方平台可通过authorizer_access_token 来代替授权账号调用接口，具体请见接口文档 |

注意：
 上述提到的所有 API 调用需要验证调用者 IP 地址。只有在第三方平台的白名单IP地址列表中填写的白名单 IP 地址列表内的 IP 地址，才能合法调用，其他一律拒绝。

## # 常见问题

### # 1、刷新令牌 authorizer_refresh_token 是需要一直保存的？有有效期这个说法吗 ？
 答：只要商家不解除授权，一直有效（解除授权-->重新授权给第三方平台，会改变 authorizer_refresh_token）
 可以调用接口getAuthorizerList获取所有已授权账号的authorizer_refresh_token。
 注意：authorizer_refresh_token返回空的情况，需要调用getAuthorizerRefreshToken接口来换取 authorizer_refresh_token
 若授权码已过期，可以触发更新授权来获取新的authorizer_refresh_token。

### # 2、出现 refresh_token is invalid 怎么办？
 首先，先自查调用getAuthorizerAccessToken请求参数 authorizer_refresh_token 是否传入了参数，有少数的开发者因给该参数传了 null 导致出现了 refresh_token is invalid 的报错。开发者可前往https://developers.weixin.qq.com/console/devtools/debug 自助查询每次请求时微信侧收到的请求参数的情况。
 如果确认 authorizer_refresh_token 传了值，那就是传入的 authorizer_refresh_token 确实是无效的。可重新调用接口getAuthorizerList获取所有已授权账号的 authorizer_refresh_token。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
