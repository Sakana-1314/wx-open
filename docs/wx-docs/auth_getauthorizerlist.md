====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/authorization-management/api_getauthorizerlist.html
====================================================================================================
# # 拉取已授权的账号信息
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getAuthorizerList
 使用本 API 拉取当前所有已授权的账号基本信息。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/component/api_get_authorizer_list?access_token=ACCESS_TOKEN
```


### # 云调用
 本接口不支持云调用。

### # 第三方调用
 本接口仅支持第三方平台使用 component_access_token 自己调用。

## # 2. 请求参数

### # 查询参数 Query String Parameters

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| access_token | string | 是 | 接口调用凭证，可使用 component_access_token |

### # 请求体 Request Payload

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| component_appid | string | 是 | 第三方平台 APPID |
| offset | number | 是 | 偏移位置/起始位置 |
| count | number | 是 | 拉取数量，最大为 500 |


## # 3. 返回参数

### # 返回体 Response Payload
 
| 参数名 | 类型 | 说明 |
|---|---|---|
| total_count | number | 授权的账号总数 |
| list | " data-v-4001728a>objarray | 当前查询的帐号基本信息列表 |
 " class="header-anchor" data-v-4001728a>

### # Res.list(Array) Object Payload
 当前查询的帐号基本信息列表

| 参数名 | 类型 | 说明 |
|---|---|---|
| authorizer_appid | string | 已授权账号的 appid |
| refresh_token | string | 刷新令牌authorizer_refresh_token |
| auth_time | number | 授权的时间 |


## # 4. 注意事项
 本接口无特殊注意事项

## # 5. 代码示例
 请求示例

```
{
  "component_appid": "appid_value",
  "offset": 0,
  "count": 100
}
```

返回示例

```
{
  "total_count": 33,
  "list": [
    {
      "authorizer_appid": "authorizer_appid_1",
      "refresh_token": "refresh_token_1",
      "auth_time": 1558000607
    },
    {
      "authorizer_appid": "authorizer_appid_2",
      "refresh_token": "refresh_token_2",
      "auth_time": 1558000607
    }
  ]
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |
| 40170 | args count exceed count limit | 个数超出限制 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
