====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/code/speedup_audit.html
====================================================================================================
## # 加急审核申请
 有加急次数的第三方可以通过该接口，对已经提审的小程序进行加急操作，加急后的小程序预计2-12小时内审完。但是，若代码中包含较复杂逻辑或其他特殊情况，可能会导致审核时间延长。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

### # 请求地址

```
POST https://api.weixin.qq.com/wxa/speedupaudit?access_token=ACCESS_TOKEN
```


### # 请求参数说明

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| access_token | String | 是 | 第三方平台接口调用令牌authorizer_access_token |
| auditid | Number | 是 | 审核单ID |

POST 数据示例:

```
{
	"auditid": 12345
}
```


### # 返回参数说明

| 参数 | 类型 | 说明 |
|---|---|---|
| errcode | Number | 返回码 |
| errmsg | String | 错误信息 |

返回结果示例：

```
{
  "errcode": 0,
  "errmsg": "ok"
}
```


### # 返回码说明

| 返回码 | 说明 |
|---|---|
| -1 | 系统繁忙，请等待修复 |
| 0 | 加急成功，请耐心等待审核结果。若代码中包含较复杂逻辑或其他特殊情况，可能会导致审核时间延长。 |
| 89401 | 系统不稳定，请稍后再试，如多次失败请通过社区反馈 |
| 89402 | 该审核单不在待审核队列，请检查是否已提交审核或已审完 |
| 89403 | 本单属于平台不支持加急种类，请等待正常审核流程 |
| 89404 | 本单已加速成功，请勿重复提交 |
| 89405 | 本月加急额度不足，请提升提审质量以获取更多额度 |
| 其他错误码 | 请查看全局错误码 |


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
