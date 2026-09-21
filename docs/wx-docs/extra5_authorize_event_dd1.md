====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/ThirdParty/token/authorize_event.html
====================================================================================================
## # 授权变更通知推送
 当公众号/小程序对第三方平台进行授权、取消授权、更新授权后，微信服务器会向第三方平台方的授权事件接收 URL（创建时由第三方平台时填写）以 POST 的方式推送相关通知，详情查看消息推送文档。
 接收 POST 请求后，只需直接返回字符串 success。

### # 字段说明

| 参数 | 类型 | 字段描述 |
|---|---|---|
| AppId | string | 第三方平台 appid |
| CreateTime | number | 时间戳 |
| InfoType | string | 通知类型，详见InfoType 说明 |
| AuthorizerAppid | string | 公众号或小程序的 appid |
| AuthorizationCode | string | 授权码，可用于获取授权信息 |
| AuthorizationCodeExpiredTime | nubmer | 授权码过期时间 单位秒 |
| PreAuthCode | string | 预授权码 |


#### # InfoType 说明

| type | 说明 |
|---|---|
| unauthorized | 取消授权 |
| updateauthorized | 更新授权 |
| authorized | 授权成功 |

推送内容解密后的示例：

##### # 授权成功通知

```
<xml>
  <AppId>第三方平台appid</AppId>
  <CreateTime>1413192760</CreateTime>
  <InfoType>authorized</InfoType>
  <AuthorizerAppid>公众号appid</AuthorizerAppid>
  <AuthorizationCode>授权码</AuthorizationCode>
  <AuthorizationCodeExpiredTime>过期时间</AuthorizationCodeExpiredTime>
  <PreAuthCode>预授权码</PreAuthCode>
<xml>
```


##### # 取消授权通知

```
<xml>
  <AppId>第三方平台appid</AppId>
  <CreateTime>1413192760</CreateTime>
  <InfoType>unauthorized</InfoType>
  <AuthorizerAppid>公众号appid</AuthorizerAppid>
</xml>
```


##### # 授权更新通知

```
<xml>
  <AppId>第三方平台appid</AppId>
  <CreateTime>1413192760</CreateTime>
  <InfoType>updateauthorized</InfoType>
  <AuthorizerAppid>公众号appid</AuthorizerAppid>
  <AuthorizationCode>授权码</AuthorizationCode>
  <AuthorizationCodeExpiredTime>过期时间</AuthorizationCodeExpiredTime>
  <PreAuthCode>预授权码</PreAuthCode>
<xml>
```


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
