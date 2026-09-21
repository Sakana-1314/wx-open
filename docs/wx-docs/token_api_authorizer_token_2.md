====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/ThirdParty/token/api_authorizer_token.html
====================================================================================================
## # 获取/刷新接口调用令牌
 在公众号/小程序接口调用令牌（authorizer_access_token）失效时，可以使用刷新令牌（authorizer_refresh_token）获取新的接口调用令牌。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。
 注意：
authorizer_access_token 有效期为 2 小时，开发者需要缓存 authorizer_access_token，避免获取/刷新接口调用令牌的 API 调用触发每日限额。缓存方法可以参考：https://developers.weixin.qq.com/doc/offiaccount/Basic_Information/Get_access_token.html

### # 请求地址

```
POST https://api.weixin.qq.com/cgi-bin/component/api_authorizer_token?component_access_token=COMPONENT_ACCESS_TOKEN
```


### # 请求参数说明

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| component_access_token | string | 是 | 第三方平台component_access_token |
| component_appid | string | 是 | 第三方平台 appid |
| authorizer_appid | string | 是 | 授权方 appid |
| authorizer_refresh_token | string | 是 | 刷新令牌，获取授权信息时得到 |

POST 数据示例：

```
{
  "component_appid": "appid_value",
  "authorizer_appid": "auth_appid_value",
  "authorizer_refresh_token": "refresh_token_value"
}
```


### # 结果参数说明

| 参数 | 类型 | 说明 |
|---|---|---|
| authorizer_access_token | string | 授权方令牌 |
| expires_in | nubmer 有效期，单位：秒 |  |
| authorizer_refresh_token | string | 刷新令牌 |

返回结果示例：

```
{
  "authorizer_access_token": "some-access-token",
  "expires_in": 7200,
  "authorizer_refresh_token": "refresh_token_value"
}
```


| 错误码 | 英文描述 | 中文描述 |
|---|---|---|
| 0 | ok | 成功 |
| 其他错误码 |  | 请查看全局错误码 |


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
