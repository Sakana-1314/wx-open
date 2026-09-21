====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/thirdparty-management/template-management/api_gettemplatelist.html
====================================================================================================
# # 获取模板列表
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getTemplateList
 通过该接口可以获取模板库里的模板列表信息。使用过程中如遇到问题，可在开放平台服务商专区发帖交流

## # 1. 调用方式

### # HTTPS 调用

```
GET https://api.weixin.qq.com/wxa/gettemplatelist?access_token=ACCESS_TOKEN
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
| template_type | number | 否 | 可选是0（对应普通模板）和1（对应标准模板），如果不填，则返回全部的。关于标准模板和普通模板的区别可查看小程序模板库介绍 |


## # 3. 返回参数

### # 返回体 Response Payload
 
| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 错误码 |
| errmsg | string | 错误信息 |
| template_list | " data-v-2d3510b2>objarray | 模板信息列表 |
 " class="header-anchor" data-v-2d3510b2>

### # Res.template_list(Array) Object Payload
 模板信息列表
 
| 参数名 | 类型 | 说明 | 枚举 |
|---|---|---|---|
| create_time | number | 开发者上传草稿时间戳 | - |
| user_version | string | 版本号，开发者自定义字段 | - |
| user_desc | string | 版本描述 开发者自定义字段 | - |
| template_id | number | 模板 id | - |
| draft_id | number | 草稿 id | - |
| source_miniprogram_appid | string | 开发小程序的appid | - |
| source_miniprogram | string | 开发小程序的名称 | - |
| template_type | number | 0对应普通模板，1对应标准模板 | - |
| category_list | __category_list" data-v-2d3510b2>objarray | 标准模板的类目信息；如果是普通模板则值为空的数组 | - |
| audit_scene | number | 标准模板的场景标签；普通模板不返回该值 | - |
| audit_status | number | 标准模板的审核状态；普通模板不返回该值 | __audit_status" data-v-2d3510b2>枚举值 |
| reason | string | 标准模板的审核驳回的原因，；普通模板不返回该值 | - |
 __category_list" class="header-anchor" data-v-2d3510b2>

### # Res.template_list(Array).category_listObject Payload
 标准模板的类目信息；如果是普通模板则值为空的数组

| 参数名 | 类型 | 说明 |
|---|---|---|
| address | string | 小程序的页面，可通过"获取小程序的页面列表getCodePage"接口获得 |
| tag | string | 小程序的标签，用空格分隔，标签至多 10 个，标签长度至多 20 |
| first_class | string | 一级类目名称，可通过"getAllCategoryName"接口获取 |
| second_class | string | 二级类目名称，可通过"getAllCategoryName"接口获取 |
| third_class | string | 三级类目名称，可通过"getAllCategoryName"接口获取 |
| title | string | 小程序页面的标题,标题长度至多 32 |
| first_id | number | 一级类目id，可通过"getAllCategoryName"接口获取 |
| second_id | number | 二级类目id，可通过"getAllCategoryName"接口获取 |
| third_id | number | 三级类目id，可通过"getAllCategoryName"接口获取 |


## # 4. 枚举信息
 __audit_status" class="header-anchor" data-v-2d3510b2>

### # Res.template_list(Array).audit_status Enum
 标准模板的审核状态；普通模板不返回该值

| 枚举值 | 描述 |
|---|---|
| 0 | 未提审核 |
| 1 | 审核中 |
| 2 | 审核驳回 |
| 3 | 审核通过 |
| 4 | 提审中 |
| 5 | 提审失败 |


## # 5. 注意事项
 请求方式是get，不是post。如果之前使用了post请求的用户，请切换成get

## # 6. 代码示例
 请求示例

```
{
  "template_type": 1
}
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "template_list": [
    {
      "create_time": 1624000154,
      "user_version": "1.0.0",
      "user_desc": "尝试提交草稿箱",
      "template_id": 47,
      "source_miniprogram_appid": "wxxxxxx",
      "source_miniprogram": "测试测试测试234",
      "developer": "。",
      "template_type": 1,
      "category_list": [
        {
          "first_class": "工具",
          "second_class": "效率",
          "first_id": 287,
          "second_id": 616
        }
      ],
      "audit_scene": 0,
      "audit_status": 2,
      "reason": ""
    },
    {
      "create_time": 1624849691,
      "user_version": "1.0.0",
      "user_desc": "尝试提交草稿箱",
      "template_id": 48,
      "source_miniprogram_appid": "wxxxxxx",
      "source_miniprogram": "测试测试测试234",
      "developer": "。",
      "template_type": 1,
      "category_list": []
    }
  ]
}
```


## # 7. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 43001 | require GET method | 需要 GET 请求 |
| 85064 | template not found | 找不到模板 |


## # 8. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
