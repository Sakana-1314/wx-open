====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/ticket-token/api_getauthorizeraccesstoken.html
====================================================================================================
# # 获取授权账号调用令牌
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getAuthorizerAccessToken
 该接口用于获取授权账号的authorizer_access_token。authorizer_access_token 有效期为 2 小时，authorizer_access_token 失效时，可以使用 authorizer_refresh_token 获取新的 authorizer_access_token。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/component/api_authorizer_token?access_token=ACCESS_TOKEN
```


### # 云调用
 本接口不支持云调用。

### # 第三方调用
 本接口仅支持第三方平台使用 component_access_token 自己调用。

## # 2. 请求参数

### # 查询参数 Query String Parameters

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| access_token | string | 是 | 接口调用凭证，可使用 component_access_token |

### # 请求体 Request Payload

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| component_appid | string | 是 | 第三方平台 appid |
| authorizer_appid | string | 是 | 授权方 appid |
| authorizer_refresh_token | string | 是 | 刷新令牌，获取授权信息时得到 |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| authorizer_access_token | string | 授权方令牌 |
| expires_in | number | 有效期，单位：秒 |
| authorizer_refresh_token | string | 刷新令牌 |


## # 4. 注意事项
 authorizer_access_token 有效期为 2 小时，开发者需要缓存 authorizer_access_token，避免 API 调用触发每日限额。
 缓存方法可以参考：1000856

## # 5. 代码示例
 请求示例

```
{
  "component_appid": "appid_value",
  "authorizer_appid": "auth_appid_value",
  "authorizer_refresh_token": "refresh_token_value"
}
```

返回示例

```
{
  "authorizer_access_token": "some-access-token",
  "expires_in": 7200,
  "authorizer_refresh_token": "refresh_token_value"
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |
| 61014 | must use component token for component api | 原因是用错了token，使用了 access_token 或者 authorizer_access_token。应该使用component_access_token调用第三方平台的 API。 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
