====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/authorization-management/api_setauthorizeroptioninfo.html
====================================================================================================
# # 设置授权方选项信息
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：setAuthorizerOptionInfo
 本 API 用于设置授权方的公众号/小程序的选项信息，如：地理位置上报，语音识别开关，多客服开关。使用过程中如遇到问题，可在开放平台服务商专区发帖交流

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/component/set_authorizer_option?access_token=ACCESS_TOKEN
```


### # 云调用
 本接口不支持云调用。

### # 第三方调用
 本接口支持第三方平台使用 component_access_token 自己调用。

## # 2. 请求参数

### # 查询参数 Query String Parameters

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| access_token | string | 是 | 接口调用凭证，可使用 component_access_token |

### # 请求体 Request Payload

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| option_name | string | 是 | 选项名称 |
| option_value | string | 是 | 设置的选项值 |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 错误码 |
| errmsg | string | 错误信息 |


## # 4. 注意事项
 注意： 设置各项选项设置信息，需要有授权方的授权，详见权限集说明。

# # 其他说明

## # option_name及option_value说明

| option_name | 选项名说明 | option_value | 选项值说明 |
|---|---|---|---|
| location_report | 地理位置上报选项 | 0 | 无上报 |
|  |  | 1 | 进入会话时上报 |
|  |  | 2 | 每5s上报 |
| voice_recognize | 语音识别开关选项 | 0 | 关闭语音识别 |
|  |  | 1 | 开启语音识别 |
| customer_service | 多客服开关选项 | 0 | 关闭多客服 |
|  |  | 1 | 开启多客服 |


## # 5. 代码示例
 请求示例

```
{
  "option_name": "option_name_value",
  "option_value": "option_value_value"
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
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
