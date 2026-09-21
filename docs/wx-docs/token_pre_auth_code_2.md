====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/ThirdParty/token/pre_auth_code.html
====================================================================================================
## # 预授权码
 预授权码（pre_auth_code）是第三方平台方实现授权托管的必备信息，每个预授权码有效期为 1800秒。需要先获取令牌才能调用。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

### # 请求地址

```
POST https://api.weixin.qq.com/cgi-bin/component/api_create_preauthcode?component_access_token=COMPONENT_ACCESS_TOKEN
```


### # 请求参数说明

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| component_access_token | string | 是 | 第三方平台component_access_token，不是authorizer_access_token |
| component_appid | string | 是 | 第三方平台 appid |

POST 数据示例：

```
{
  "component_appid": "appid_value" 
}
```


### # 结果参数说明

| 参数 | 类型 | 说明 |
|---|---|---|
| pre_auth_code | string | 预授权码 |
| expires_in | number | 有效期，单位：秒 |

返回结果示例：

```
{
  "pre_auth_code": "Cx_Dk6qiBE0Dmx4EmlT3oRfArPvwSQ-oa3NL_fwHM7VI08r52wazoZX2Rhpz1dEw",
  "expires_in": 600
}
```


### # 返回码说明

| 错误码 | 英文描述 | 中文描述 |
|---|---|---|
| 61004 | access clientip is not registered |  |
| 61005 | component ticket is expired |  |
| 41004 | appsecret missing | 缺少 secret 参数 |
| 40125 | invalid appsecret | 无效的appsecret |
| 61006 | component ticket is invalid |  |
| 61011 | invalid component |  |
| 45009 | reach max api daily quota limit | 接口调用超过限制 |
| 47001 | data format error | 解析 JSON/XML 内容错误 |
| 40001 | invalid credential, access_token is invalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 48001 | api unauthorized | api 功能未授权，请确认公众号/小程序已获得该接口，可以在公众平台官网 - 开发者中心页中查看接口权限 |
| 42001 | access_token expired | access_token 超时，请检查 access_token 的有效期，请参考基础支持 - 获取 access_token 中，对 access_token 的详细机制说明 |
| 40001 | invalid credential, access_token is invalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 41001 | access_token missing | 缺少 access_token 参数 |
| 61004 | access clientip is not registered |  |
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |
| 61011 | invalid component |  |
| 48001 | api unauthorized | api 功能未授权，请确认公众号已获得该接口，可以在公众平台官网 - 开发者中心页中查看接口权限 |
| 其他错误码 |  | 请查看全局错误码 |


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
