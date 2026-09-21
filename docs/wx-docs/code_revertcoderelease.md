====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_revertcoderelease.html
====================================================================================================
# # 小程序版本回退
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：revertCodeRelease
 调用本接口可以将小程序的线上版本进行回退。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
GET https://api.weixin.qq.com/wxa/revertcoderelease?access_token=ACCESS_TOKEN
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
| action | string | 否 | 只能填get_history_version。表示获取可回退的小程序版本。该参数为 URL 参数，非 Body 参数。 |
| app_version | string | 否 | 默认是回滚到上一个版本；也可回滚到指定的小程序版本，可通过get_history_version获取app_version。该参数为 URL 参数，非 Body 参数。 |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 错误码 |
| errmsg | string | 错误信息 |
| version_list | object | 模板信息列表。当action=get_history_version，才会返回。 |


### # Res.version_list Object Payload
 模板信息列表。当action=get_history_version，才会返回。

| 参数名 | 类型 | 说明 |
|---|---|---|
| app_version | number | 小程序版本 |
| user_version | string | 模板版本号，开发者自定义字段 |
| user_desc | string | 模板描述，开发者自定义字段 |
| commit_time | number | 更新时间，时间戳 |


## # 4. 注意事项
 如果没有上一个线上版本，将无法回退
 可指定版本进行回滚，但最多保存最近发布或回退的5个版本
 当前版本回退后，不能再调用版本回退接口，也不会再查到版本信息。

## # 5. 代码示例

### # 5.1 小程序版本回退
 请求示例

```
GET https://api.weixin.qq.com/wxa/revertcoderelease?app_version=123&access_token=ACCESS_TOKEN
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok"
}
```


### # 5.2 获取可回退的小程序版本
 请求示例

```
GET https://api.weixin.qq.com/wxa/revertcoderelease?action=get_history_version&access_token=ACCESS_TOKEN
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "version_list": [
    {
      "commit_time": 1488965944,
      "user_version": "VVV",
      "user_desc": "AAS",
      "app_version": 111
    },
    {
      "commit_time": 1504790906,
      "user_version": "11",
      "user_desc": "111111",
      "app_version": 222
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
| 40097 | invalid args | 参数错误 |
| 87012 | forbid revert this version release | 该版本不能回退，可能的原因：1:无上一个线上版用于回退 2:此版本为已回退版本，不能回退 3:此版本为回退功能上线之前的版本，不能回退 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
