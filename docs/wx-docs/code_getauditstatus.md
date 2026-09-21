====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getauditstatus.html
====================================================================================================
# # 查询审核单状态
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getAuditStatus
 通过接口提交代码审核后，调用本接口可以查询指定发布审核单的审核状态。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/wxa/get_auditstatus?access_token=ACCESS_TOKEN
```


### # 云调用
 本接口不支持云调用。

### # 第三方调用
 本接口支持第三方平台代商家调用。

 该接口所属的权限集 id 为：18

 服务商获得其中之一权限集授权后，可通过使用 authorizer_access_token 代商家进行调用，具体可查看 第三方调用 说明文档。

## # 2. 请求参数

### # 查询参数 Query String Parameters

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| access_token | string | 是 | 接口调用凭证，可使用 authorizer_access_token |

### # 请求体 Request Payload

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| auditid | number | 是 | 提交审核时获得的审核 id |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 返回码 |
| errmsg | string | 错误信息 |
| status | number | 审核状态 |
| reason | string | 当 status = 1 时，返回的拒绝原因; status = 4 时，返回的延后原因 |
| screenshot | string | 当 status = 1 时，会返回审核失败的小程序截图示例。用竖线分隔的 media_id 的列表，可通过获取永久素材接口拉取截图内容 |


## # 4. 注意事项

# # 其他说明
 请注意调用如下接口时需要使用第三方平台接口调用令牌authorizer_access_token
 获取永久素材接口

## # 审核状态说明

| 状态值 | 说明 |
|---|---|
| 0 | 审核成功 |
| 1 | 审核被拒绝 |
| 2 | 审核中 |
| 3 | 已撤回 |
| 4 | 审核延后 |


## # 5. 代码示例
 请求示例

```
{
  "auditid": 1234567
}
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "status": 1,
  "reason": "帐号信息不合规范",
  "screenshot": "xxx|yyy|zzz"
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 85012 | invalid audit id | 无效的审核 id |
| 86000 | should be called only from third party | 不是由第三方代小程序进行调用 |
| 86001 | component experience version not exists | 不存在第三方的已经提交的代码 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
