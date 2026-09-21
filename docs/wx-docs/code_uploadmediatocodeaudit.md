====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_uploadmediatocodeaudit.html
====================================================================================================
# # 上传提审素材
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：uploadMediaToCodeAudit
 调用本接口可将小程序页面截图和操作录屏上传，提审时带上相关参数，可以帮助审核人员判断

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/wxa/uploadmedia?access_token=ACCESS_TOKEN
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
| media | - | 是 | 图片（image）: 2M，支持PNG\JPEG\JPG\GIF格式 视频（video）：10MB，支持MP4格式 完成素材上传后，使用返回的mediaid，可以在提审接口通过post preview_info完成图片和视频上传。 注意：返回的 mediaid 有效期是三天，过期需要重新上传 |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 错误码 |
| errmsg | string | 错误信息 |
| type | string | 类型 |
| mediaid | buffer | 媒体id |


## # 4. 注意事项
 本接口无特殊注意事项

## # 5. 代码示例

### # 5.1 传图片示例
 请求示例

```
curl -F media=@test.jpg "https://api.weixin.qq.com/wxa/uploadmedia?access_token=ACCESS_TOKEN"
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "type": "image",
  "mediaid": "xxxxxxxxxxxxxxxxx"
}
```


### # 5.2 传mp4示例
 请求示例

```
curl -F media=@xxx.mp4 "https://api.weixin.qq.com/wxa/uploadmedia?access_token=ACCESS_TOKEN"
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "type": "video",
  "mediaid": "xxxxxxxxxxxxxxxxx"
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| 40005 | invalid file type | 上传素材文件格式不对 |
| 40006 | invalid meida size | 上传素材文件大小超出限制 |
| 41005 | media data missing | 缺少多媒体文件数据，传输素材无视频或图片内容 |
| 43002 | require POST method | 需要 POST 请求 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
