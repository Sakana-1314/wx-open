====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/login/api_thirdpartycode2session.html
====================================================================================================
# # 小程序登录
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：thirdpartyCode2Session
 第三方平台开发者的服务器使用登录凭证code以及第三方平台的 component_access_token 可以代替小程序实现登录功能 获取 session_key 和 openid。
 其中 session_key 是对用户数据进行加密签名的密钥。为了自身应用安全，session_key 不应该在网络上传输。

## # 1. 调用方式

### # HTTPS 调用

```
GET https://api.weixin.qq.com/sns/component/jscode2session?appid=APPID&grant_type=GRANT_TYPE&component_appid=COMPONENT_APPID&component_access_token=COMPONENT_ACCESS_TOKEN&js_code=JS_CODE
```


### # 云调用
 本接口不支持云调用。

### # 第三方调用
 本接口仅支持第三方平台使用。

## # 2. 请求参数

### # 查询参数 Query String Parameters

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| appid | string | 是 | 小程序的 AppID |
| grant_type | string | 是 | 填 authorization_code |
| component_appid | string | 是 | 第三方平台 appid |
| component_access_token | string | 是 | 接口调用凭证，该参数为 URL 参数，非 Body 参数。可通过获取令牌接口获取。 |
| js_code | string | 是 | wx.login 获取的 code |

### # 请求体 Request Payload
 无

## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| session_key | string | 会话密钥 |
| unionid | string | 用户在开放平台的唯一标识符，若当前小程序已绑定到微信开放平台账号下会返回，详见 UnionID 机制说明。 |
| openid | string | 用户唯一标识 |
| errcode | number | 错误码，请求失败时返回 |
| errmsg | string | 错误信息，请求失败时返回 |


## # 4. 注意事项
 该接口仅支持服务商获取已有授权关系对的小程序的信息，若小程序尚未授权给第三方平台，则会出现61003报错。

## # 5. 代码示例
 请求示例

```
GET https://api.weixin.qq.com/sns/component/jscode2session?appid=APPID&js_code=JSCODE&grant_type=authorization_code&component_appid=COMPONENT_APPID&component_access_token=COMPONENT_ACCESS_TOKEN
```

返回示例

```
{
  "openid": "OPENID",
  "session_key": "SESSIONKEY",
  "unionid": "oHAUs6LSuwgHq-mlnFrffKXw3QYM"
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| 40029 | code 无效 | js_code无效 |
| 41021 | missing component_access_token |  |
| 45011 | api minute-quota reach limit mustslower retry next minute | API 调用太频繁，请稍候再试 |
| 61003 | component is not authorized by this account |  |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
