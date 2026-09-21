====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getvisitstatus.html
====================================================================================================
# # 查询小程序服务状态
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getVisitStatus
 调用本接口可以查询小程序的服务状态。补充说明：该接口控制的服务状态信息对应的是mp控制台上的“暂停服务设置”所控制的小程序服务状态。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/wxa/getvisitstatus?access_token=ACCESS_TOKEN
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
 无

## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 返回码 |
| errmsg | string | 错误信息 |
| status | number | 服务状态。0表示已暂停服务（包含主动暂停服务违规被暂停服务）。1表示未暂停服务。 |


## # 4. 注意事项
 要传空的json，不传会报错44002

## # 5. 代码示例
 请求示例

```
{}
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "status": 1
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 44002 | empty post data | POST 的数据包为空。post请求body参数不能为空。 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
