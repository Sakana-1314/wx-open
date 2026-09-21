====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/ThirdParty/token/component_verify_ticket.html
====================================================================================================
## # 验证票据
 验证票据（component_verify_ticket），在第三方平台创建审核通过后，微信服务器会向其 ”授权事件接收URL” 每隔 10 分钟以 POST 的方式推送 component_verify_ticket，详情查看消息推送文档。
 接收 POST 请求后，只需直接返回字符串 success。
 注意：
 component_verify_ticket 的有效时间为12小时，比 component_access_token 更长，建议保存最近可用的component_verify_ticket，在 component_access_token 过期之前都可以直接使用该 component_verify_ticket 进行更新，避免出现因为 component_verify_ticket 接收失败而无法更新 component_access_token 的情况。

### # 参数说明

| 参数 | 类型 | 字段描述 |
|---|---|---|
| AppId | string | 第三方平台 appid |
| CreateTime | number | 时间戳，单位：s |
| InfoType | string | 固定为："component_verify_ticket" |
| ComponentVerifyTicket | string | Ticket 内容 |

推送内容解密后的示例：

```
<xml>
<AppId>some_appid</AppId>
<CreateTime>1413192605</CreateTime>
<InfoType>component_verify_ticket</InfoType>
<ComponentVerifyTicket>some_verify_ticket</ComponentVerifyTicket>
</xml>
```


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
