====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/openapi/api_clearquota.html
====================================================================================================
# # 重置API调用次数
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：clearQuota
 本接口是通过access_token清空服务端接口的每日调用接口次数。
 适用的账号类型为：公众号/服务号/小程序/小游戏/微信小店/带货助手/视频号助手/联盟带货机构/移动应用/网站应用/多端应用/第三方平台

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/clear_quota?access_token=ACCESS_TOKEN
```

支持加密请求： 本接口支持服务通信二次加密和签名，可有效防止数据篡改与泄露。查看详情

### # 云调用
 本接口不支持云调用。

### # 第三方调用
 本接口支持第三方平台使用 component_access_token 自己调用，同时还支持代商家调用。

 服务商获得任意权限集授权后，即可通过使用 authorizer_access_token 代商家进行调用，具体可查看 第三方调用 说明文档。

## # 2. 请求参数

### # 查询参数 Query String Parameters

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| access_token | string | 是 | 接口调用凭证，可使用 access_token、component_access_token、authorizer_access_token |

### # 请求体 Request Payload

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| appid | string | 是 | 要被清空的账号的appid |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 错误码 |
| errmsg | string | 错误信息 |


## # 4. 注意事项
 1、如果要清空公众号的接口的quota，则需要用公众号的access_token；如果要清空小程序的接口的quota，则需要用小程序的access_token；如果要清空第三方平台的接口的quota，则需要用第三方平台的component_access_token。如此类推。
 2、如果是第三方服务商代公众号/服务号/小程序/微信小店/带货助手/视频号助手清除quota，则需要用authorizer_access_token
 3、每个账号每月共10次清零操作机会，清零生效一次即用掉一次机会；第三方帮公众号/服务号/小程序/微信小店/带货助手/视频号助手代调用时，实际上是在消耗公众号/服务号/小程序/微信小店/带货助手/视频号助手自身的quota
 4、由于指标计算方法或统计时间差异，实时调用量数据可能会出现误差，一般在1%以内

## # 5. 代码示例
 请求示例

```
{
  "appid": "wx448f04719cd48f69"
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
| 0 | ok | ok |
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |
| 48006 | forbid to clear quota because of reaching the limit | api 禁止清零调用次数，因为清零次数达到上限 |


## # 7. 适用范围
 本接口在不同账号类型下的可调用情况：

| 小程序 | 公众号 | 服务号 | 小游戏 | 微信小店 | 联盟带货机构 | 带货助手 | 小店供货商 | 第三方平台 | 移动应用 | 网站应用 | 视频号助手 | 多端应用 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | 〇 | ✔ | ✔ | ✔ | ✔ |

 ✔：该账号可调用此接口。
〇：第三方平台可使用 component_access_token 调用，是否支持代商家调用需看本文档 调用方式 部分。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
