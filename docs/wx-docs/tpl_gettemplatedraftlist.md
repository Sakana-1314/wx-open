====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/thirdparty-management/template-management/api_gettemplatedraftlist.html
====================================================================================================
# # 获取草稿箱列表
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getTemplatedRaftList
 通过本接口，可以获取第三方平台草稿箱中所有的草稿；
 说明，草稿是由第三方平台的开发小程序在使用微信开发者工具上传的。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
GET https://api.weixin.qq.com/wxa/gettemplatedraftlist?access_token=ACCESS_TOKEN
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
 无

## # 3. 返回参数

### # 返回体 Response Payload
 
| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 错误码 |
| errmsg | string | 错误信息 |
| draft_list | " data-v-0b2f061a>objarray | 草稿箱信息。 |
 " class="header-anchor" data-v-0b2f061a>

### # Res.draft_list(Array) Object Payload
 草稿箱信息。

| 参数名 | 类型 | 说明 |
|---|---|---|
| create_time | number | 开发者上传草稿时间戳 |
| user_version | string | 版本号，开发者自定义字段 |
| user_desc | string | 版本描述 开发者自定义字段 |
| draft_id | number | 草稿 id |
| source_miniprogram_appid | string | 开发小程序的appid |
| source_miniprogram | string | 开发小程序的名称 |
| developer | string | 操作者微信昵称 |


## # 4. 注意事项
 本接口无特殊注意事项

## # 5. 代码示例
 请求示例

```
GET https://api.weixin.qq.com/wxa/gettemplatedraftlist?access_token=ACCESS_TOKEN
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "draft_list": [
    {
      "create_time": 1488965944,
      "user_version": "VVV",
      "user_desc": "AAS",
      "draft_id": 0,
      "source_miniprogram_appid": "wx6XXXXXXXXXXXXXXX",
      "source_miniprogram": "LXXXXX",
      "developer": "XXX",
      "category_list": []
    },
    {
      "create_time": 1504790906,
      "user_version": "11",
      "user_desc": "111111",
      "draft_id": 4,
      "source_miniprogram_appid": "wx6XXXXXXXXXXXXXXX",
      "source_miniprogram": "LXXXXX",
      "developer": "XXX",
      "category_list": []
    }
  ]
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 85064 | template not found | 找不到模板 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
