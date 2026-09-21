====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_getgrayreleaseplan.html
====================================================================================================
# # 获取分阶段发布详情
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getGrayReleasePlan
 该接口用于查询当前分阶段发布详情

## # 1. 调用方式

### # HTTPS 调用

```
GET https://api.weixin.qq.com/wxa/getgrayreleaseplan?access_token=ACCESS_TOKEN
```


### # 云调用
 调用方法：operation.getGrayReleasePlan

 出入参和 HTTPS 调用相同，调用方式可查看 云调用 说明文档。

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
| errcode | number | 错误码 |
| errmsg | string | 错误时间 |
| gray_release_plan | object | 分阶段发布计划详情 |


### # Res.gray_release_plan Object Payload
 分阶段发布计划详情

| 参数名 | 类型 | 说明 |
|---|---|---|
| status | number | 0:初始状态 1:执行中 2:暂停中 3:执行完毕 4:被删除 |
| create_timestamp | number | 分阶段发布计划的创建时间 |
| gray_percentage | number | 当前的灰度比例 |
| support_debuger_first | boolean | true表示支持按项目成员灰度 |
| support_experiencer_first | boolean | true表示支持按体验成员灰度 |


## # 4. 注意事项
 本接口无特殊注意事项

## # 5. 代码示例
 请求示例

```
GET https://api.weixin.qq.com/wxa/getgrayreleaseplan?access_token=ACCESS_TOKEN
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "gray_release_plan": {
    "status": 1, //
    "create_timestamp": 1517553721, //创建时间
    "gray_percentage": 8,
    "support_experiencer_first": true,
    "support_debuger_first": true
  }
```

}

## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
