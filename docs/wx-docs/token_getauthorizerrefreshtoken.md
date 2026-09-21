====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/ticket-token/api_getauthorizerrefreshtoken.html
====================================================================================================
# # 获取刷新令牌
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getAuthorizerRefreshToken
 当用户在第三方平台授权页中完成授权流程后，第三方平台开发者可以在回调 URI 中通过 URL 参数获取授权码(authorization_code)。然后使用该接口可以换取公众号/小程序的刷新令牌（authorizer_refresh_token）。
 建议保存授权信息中的刷新令牌（authorizer_refresh_token）使用过程中如遇到问题，可在开放平台服务商专区发帖交流

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/component/api_query_auth?access_token=ACCESS_TOKEN
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
| authorization_code | string | 是 | 授权码, 会在授权成功时返回给第三方平台，详见第三方平台授权流程说明。该参数也可以通过平台推送的"授权变更通知"获取。 |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| authorization_info | object | 授权信息 |


### # Res.authorization_info Object Payload
 授权信息
 
| 参数名 | 类型 | 说明 |
|---|---|---|
| authorizer_appid | string | 授权的公众号或者小程序 appid |
| authorizer_access_token | string | 接口调用令牌（在授权的公众号/小程序具备 API 权限时，才有此返回值） |
| expires_in | number | authorizer_access_token 的有效期（在授权的公众号/小程序具备API权限时，才有此返回值），单位：秒 |
| authorizer_refresh_token | string | 刷新令牌（在授权的公众号具备API权限时，才有此返回值），刷新令牌主要用于第三方平台获取和刷新已授权用户的 authorizer_access_token。一旦丢失，只能让用户重新授权，才能再次拿到新的刷新令牌。用户重新授权后，之前的刷新令牌会失效 |
| func_info | " data-v-68f90c65>objarray | 授权给第三方平台的权限集id列表，权限集id代表的含义可查看权限集介绍 |
 " class="header-anchor" data-v-68f90c65>

### # Res.authorization_info.func_info(Array) Object Payload
 授权给第三方平台的权限集id列表，权限集id代表的含义可查看权限集介绍
 
| 参数名 | 类型 | 说明 |
|---|---|---|
| funcscope_category | __funcscope_category" data-v-68f90c65>object | 授权给开发者的权限集详情 |
 __funcscope_category" class="header-anchor" data-v-68f90c65>

### # Res.authorization_info.func_info(Array).funcscope_category Object Payload
 授权给开发者的权限集详情

| 参数名 | 类型 | 说明 |
|---|---|---|
| id | number | 权限集id |
| type | number | 权限集类型 |
| name | string | 权限集名称 |
| desc | string | 权限集描述 |


## # 4. 注意事项
 公众号/小程序可以自定义选择部分权限授权给第三方平台，因此第三方平台开发者需要通过该接口来获取公众号/小程序具体授权了哪些权限，而不是简单地认为自己声明的权限就是公众号/小程序授权的权限。
 刷新令牌 authorizer_refresh_token 有效期：参考生成说明-常见问题

## # 5. 代码示例
 请求示例

```
{
  "component_appid": "appid_value",
  "authorization_code": "auth_code_value"
}
```

返回示例

```
{
  "authorization_info": {
    "authorizer_appid": "wxf8b4f85f3a794e77",
    "authorizer_access_token": "QXjUqNqfYVH0yBE1iI_7vuN_9gQbpjfK7hYwJ3P7xOa88a89-Aga5x1NMYJyB8G2yKt1KCl0nPC3W9GJzw0Zzq_dBxc8pxIGUNi_bFes0qM",
    "expires_in": 7200,
    "authorizer_refresh_token": "dTo-YCXPL4llX-u1W1pPpnp8Hgm4wpJtlR6iV0doKdY",
    "func_info": [
      {
        "funcscope_category": {
          "id": 1
        }
      },
      {
        "funcscope_category": {
          "id": 2
        }
      },
      {
        "funcscope_category": {
          "id": 3
        }
      }
    ]
  }
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


## # 7. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
