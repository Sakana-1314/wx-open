====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getsupportversion.html
====================================================================================================
# # 查询各版本用户占比
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getSupportVersion
 调用本接口可以查询小程序当前设置的最低基础库版本，以及小程序在各个基础库版本的用户占比。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/wxopen/getweappsupportversion?access_token=ACCESS_TOKEN
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
| now_version | string | 当前版本 |
| uv_info | object | 版本的用户占比列表 |


### # Res.uv_info Object Payload
 版本的用户占比列表
 
| 参数名 | 类型 | 说明 |
|---|---|---|
| items | " data-v-20f8dd30>objarray | 版本的用户占比列表 |
 " class="header-anchor" data-v-20f8dd30>

### # Res.uv_info.items(Array) Object Payload
 版本的用户占比列表

| 参数名 | 类型 | 说明 |
|---|---|---|
| version | string | 基础库版本号 |
| percentage | number | 该版本用户占比 |


## # 4. 注意事项
 要传空的json，不传会报错

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
  "now_version": "1.0.0",
  "uv_info": {
    "items": [
      {
        "percentage": 0,
        "version": "1.0.0"
      },
      {
        "percentage": 0,
        "version": "1.0.1"
      },
      {
        "percentage": 0,
        "version": "1.1.0"
      }
    ]
  }
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| 0 | ok | ok |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
