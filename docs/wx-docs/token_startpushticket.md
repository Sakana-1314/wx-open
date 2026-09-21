====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/ticket-token/api_startpushticket.html
====================================================================================================
# # 启动票据推送服务
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：startPushTicket
 该 API 用于启动ticket推送服务。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/component/api_start_push_ticket
```


### # 云调用
 本接口不支持云调用。

### # 第三方调用
 本接口仅支持第三方平台使用。

## # 2. 请求参数

### # 查询参数 Query String Parameters
 无

### # 请求体 Request Payload

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| component_appid | string | 是 | 平台型第三方平台的appid |
| component_secret | string | 是 | 平台型第三方平台的APPSECRET |


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
  "component_appid": "wxd0bfc95064b5bfc1",
  "component_secret": "28c223a63f7de0d5xxxxxxxxxxx"
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
| 0 | ok或者in a normal state | ok是指从不正常变成正常 in a normal state是指本来就正常 |
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
