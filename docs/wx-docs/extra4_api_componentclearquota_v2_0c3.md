====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/openapi/api_componentclearquota_v2.html
====================================================================================================
# # 使用AppSecret重置第三方平台API调用次数
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：clearComponentQuotaByAppSecret
 本接口用于清空公众号/小程序/第三方等接口的每日调用接口次数

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/component/clear_quota/v2
```


### # 云调用
 本接口不支持云调用。

### # 第三方调用
 本接口仅支持第三方平台使用 component_access_token 自己调用。

## # 2. 请求参数

### # 查询参数 Query String Parameters
 无

### # 请求体 Request Payload

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| appid | string | 是 | 授权用户appid |
| component_appid | string | 是 | 第三方appid |
| appsecret | string | 是 | 第三方appsecret |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 错误码 |
| errmsg | string | 错误信息 |


## # 4. 注意事项
 1、该接口通过appsecret调用，解决了accesss_token耗尽无法调用重置 API 调用次数
的情况
 2、每个帐号每月使用重置 API 调用次数
 与本接口共10次清零操作机会，清零生效一次即用掉一次机会；
 3、由于指标计算方法或统计时间差异，实时调用量数据可能会出现误差，一般在1%以内
 4、该接口仅支持POST调用
 5、该接口可以代公众号/小程序用户重置接口调用次数，appid参数需要是已授权的账号。

## # 5. 代码示例

### # 5.1 重置第三方账号API调用次数
 请求示例
 POST  https://api.weixin.qq.com/cgi-bin/component/clear_quota/v2

```
{
    "component_appid":"wxxxxxxxxxxxxxxxxxx",
    "appsecret":"xxxxxxxxxxxxxxxxxxxxxxxx"
}
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok"
}
```


### # 5.2 代公众号/小程序重置API调用次数
 请求示例
 POST  https://api.weixin.qq.com/cgi-bin/component/clear_quota/v2

```
{
    "appid":"wxxxxxxxxxxxxxxxxxx",
    "component_appid":"wxxxxxxxxxxxxxxxxxx",
    "appsecret":"xxxxxxxxxxxxxxxxxxxxxxxx"
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
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |
| 41002 | appid missing | 缺少 appid 参数 |
| 41004 | appsecret missing | 缺少 secret 参数 |
| 43007 | require bizuser authorize | 检查授权关系 |
| 48006 | forbid to clear quota because of reaching the limit | api 禁止清零调用次数，因为清零次数达到上限 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
