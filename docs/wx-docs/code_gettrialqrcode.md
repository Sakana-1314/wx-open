====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_gettrialqrcode.html
====================================================================================================
# # 获取体验版二维码
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getTrialQRCode
 调用本接口可以获取小程序的体验版二维码。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
GET https://api.weixin.qq.com/wxa/get_qrcode?access_token=ACCESS_TOKEN
```


### # 云调用
 本接口不支持云调用。

### # 第三方调用
 本接口支持第三方平台代商家调用。

 该接口所属的权限集 id 为：18、86

 服务商获得其中之一权限集授权后，可通过使用 authorizer_access_token 代商家进行调用，具体可查看 第三方调用 说明文档。

## # 2. 请求参数

### # 查询参数 Query String Parameters

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| access_token | string | 是 | 接口调用凭证，可使用 access_token、authorizer_access_token |

### # 请求体 Request Payload

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| path | string | 否 | 指定二维码扫码后直接进入指定页面并可同时带上参数 |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 返回码 |
| errmsg | string | 错误信息 |


## # 4. 注意事项

# # 其他说明
 path 需要进行一次 urlencode，如：page/index?action=1，需要填入 page%2Findex%3Faction%3D1

## # 返回说明
 请求正常的情况下，开发者可以直接将返回的二进制结果（response body）保存成图片。返回的 HTTP 头如下：

```
HTTP/1.1 200 OK

Connection: close

Content-Type: image/jpeg

Content-disposition: attachment; filename="QRCode.jpg"

Date: Sun, 06 Jan 2013 10:20:18 GMT

Cache-Control: no-cache, must-revalidate

Content-Length: 339721
```


## # 5. 代码示例
 请求示例

```
https://api.weixin.qq.com/wxa/get_qrcode?access_token=ACCESS_TOKEN&path=page/index?action=1
```

返回示例

```
{
  "errcode": -1,
  "errmsg": "system error"
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 40014 | invalid access_token | 不合法的 access_token ，请开发者认真比对 access_token 的有效性（如是否过期），或查看是否正在为恰当的公众号调用接口 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
