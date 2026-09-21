====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_grayrelease.html
====================================================================================================
# # 分阶段发布
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：grayRelease
 发布小程序release接口是全量发布，会影响到现网的所有用户。而本接口是创建一个灰度发布的计划，可以控制发布的节奏，避免一上线就影响到所有的用户。可以多次调用本次接口，将灰度的比例（gray_percentage）逐渐增大
 使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/wxa/grayrelease?access_token=ACCESS_TOKEN
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
| gray_percentage | number | 是 | 灰度的百分比 0 ~ 100 的整数。如果gray_percentage=0，support_experiencer_first与support_debuger_first二选一必填 |
| support_debuger_first | boolean | 否 | true表示支持按项目成员灰度，默认是false |
| support_experiencer_first | boolean | 否 | true表示支持按体验成员灰度，默认是false |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 错误码 |
| errmsg | string | 错误信息 |


## # 4. 注意事项
 本接口无特殊注意事项

## # 5. 代码示例
 请求示例

```
{
  "gray_percentage": 1,
  "support_experiencer_first": true,
  "support_debuger_first": true
}
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok"
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 40097 | invalid args | 参数错误 |
| 85079 | miniprogram has no online release | 小程序没有线上版本，即小程序尚未发布，不可进行该操作 |
| 85080 | miniprogram commit not approved | 小程序提交的审核未审核通过 |
| 85081 | invalid gray percentage | 无效的发布比例 |
| 85082 | gray percentage too low | 当前的发布比例需要比之前设置的高 |
| 86002 | miniprogram have not completed init procedure | 小程序还未设置昵称、头像、简介。请先设置完后再重新提交 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
