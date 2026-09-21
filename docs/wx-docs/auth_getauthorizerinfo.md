====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/authorization-management/api_getauthorizerinfo.html
====================================================================================================
# # 获取授权账号详情
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getAuthorizerInfo
 该 API 用于获取授权方的基本信息，包括头像、昵称、账号类型、认证类型、原始ID等信息。使用过程中如遇到问题，可在开放平台服务商专区发帖交流

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/component/api_get_authorizer_info?access_token=ACCESS_TOKEN
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
| component_appid | string | 是 | 第三方平台 appid |
| authorizer_appid | string | 是 | 授权的公众号或者小程序的appid |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| authorizer_info | object | 授权账号信息 |
| authorization_info | object | 授权信息 |


### # Res.authorizer_info Object Payload
 授权账号信息

| 参数名 | 类型 | 说明 | 枚举 |
|---|---|---|---|
| nick_name | string | 昵称 | - |
| head_img | string | 头像 | - |
| service_type_info | object | 公众号/小程序类型 | - |
| verify_type_info | object | 公众号/小程序认证类型 | - |
| user_name | string | 原始 ID | - |
| alias | string | 公众号所设置的微信号，可能为空 | - |
| qrcode_url | string | 二维码图片的 URL | - |
| business_info | object | 用以了解功能的开通状况（0代表未开通，1代表已开通） | - |
| idc | number | 废弃参数 | - |
| principal_name | string | 主体名称 | - |
| signature | string | 小程序账号介绍 | - |
| MiniProgramInfo | object | 小程序配置信息，可根据这个字段判断是否为小程序类型授权 | - |
| register_type | number | 小程序注册方式 | 枚举值 |
| account_status | number | 账号状态，该字段小程序也返回 | 枚举值 |
| basic_config | object | 基础配置信息 | - |
| channels_info | object | 视频号账号类型；如果该授权账号为视频号则返回该字段 | - |
| store_info | object | 小店账号类型；如果该授权账号为小店则返回该字段 | - |
| talent_info | object | 带货助手账号类型；如果该授权账号为带货助手则返回该字段 | - |
| supplier_info | object | 供货商账号类型；如果该授权账号为供货商则返回该字段 | - |


### # Res.authorization_info Object Payload
 授权信息

| 参数名 | 类型 | 说明 |
|---|---|---|
| authorizer_appid | string | 授权的公众号或者小程序 appid |
| authorizer_refresh_token | string | 刷新令牌（在授权的公众号具备API权限时，才有此返回值），刷新令牌主要用于第三方平台获取和刷新已授权用户的 authorizer_access_token。一旦丢失，只能让用户重新授权，才能再次拿到新的刷新令牌。用户重新授权后，之前的刷新令牌会失效 |
| func_info | " data-v-90159960>objarray | 授权给第三方平台的权限集id列表，权限集id的含义可查看权限集介绍 |


### # Res.authorizer_info.service_type_info Object Payload
 公众号/小程序类型

| 参数名 | 类型 | 说明 | 枚举 |
|---|---|---|---|
| id | number | 类型id | 枚举值 |
| name | string | 类型说明 | - |


### # Res.authorizer_info.verify_type_info Object Payload
 公众号/小程序认证类型

| 参数名 | 类型 | 说明 | 枚举 |
|---|---|---|---|
| id | number | 类型id | 枚举值 |
| name | string | 类型说明 | - |


### # Res.authorizer_info.business_info Object Payload
 用以了解功能的开通状况（0代表未开通，1代表已开通）

| 参数名 | 类型 | 说明 |
|---|---|---|
| open_pay | number | 是否开通微信支付功能 |
| open_shake | number | 是否开通微信摇一摇功能 |
| open_scan | number | 是否开通微信扫商品功能 |
| open_card | number | 是否开通微信卡券功能 |
| open_store | number | 是否开通微信门店功能 |


### # Res.authorizer_info.MiniProgramInfo Object Payload
 小程序配置信息，可根据这个字段判断是否为小程序类型授权

| 参数名 | 类型 | 说明 |
|---|---|---|
| network | object | 小程序配置的合法域名信息 |
| categories | " data-v-90159960>objarray | 小程序配置的类目信息 |
| visit_status | number | 废弃参数 |


### # Res.authorizer_info.MiniProgramInfo.network Object Payload
 小程序配置的合法域名信息
 
| 参数名 | 类型 | 说明 |
|---|---|---|
| RequestDomain | array | request合法域名 |
| WsRequestDomain | array | socket合法域名 |
| UploadDomain | array | uploadFile合法域名 |
| DownloadDomain | array | downloadFile合法域名 |
| UDPDomain | array | udp合法域名 |
| TCPDomain | array | tcp合法域名 |
 " class="header-anchor" data-v-90159960>

### # Res.authorizer_info.MiniProgramInfo.categories(Array) Object Payload
 小程序配置的类目信息

| 参数名 | 类型 | 说明 |
|---|---|---|
| first | string | 一级类目 |
| second | string | 二级类目 |


### # Res.authorizer_info.basic_config Object Payload
 基础配置信息

| 参数名 | 类型 | 说明 |
|---|---|---|
| is_phone_configured | boolean | 是否已经绑定手机号 |
| is_email_configured | boolean | 是否已经绑定邮箱，不绑定邮箱账号的不可登录微信公众平台 |


### # Res.authorizer_info.channels_info Object Payload
 视频号账号类型；如果该授权账号为视频号则返回该字段

| 参数名 | 类型 | 说明 | 枚举 |
|---|---|---|---|
| id | number | 视频号类型 | 枚举值 |


### # Res.authorizer_info.store_info Object Payload
 小店账号类型；如果该授权账号为小店则返回该字段

| 参数名 | 类型 | 说明 | 枚举 |
|---|---|---|---|
| id | number | 小店类型 | 枚举值 |


### # Res.authorizer_info.talent_info Object Payload
 带货助手账号类型；如果该授权账号为带货助手则返回该字段

| 参数名 | 类型 | 说明 | 枚举 |
|---|---|---|---|
| id | number | 带货助手类型 | 枚举值 |


### # Res.authorizer_info.supplier_info Object Payload
 供货商账号类型；如果该授权账号为供货商则返回该字段
 
| 参数名 | 类型 | 说明 | 枚举 |
|---|---|---|---|
| id | number | 供货商类型 | 枚举值 |
 " class="header-anchor" data-v-90159960>

### # Res.authorization_info.func_info(Array) Object Payload
 授权给第三方平台的权限集id列表，权限集id的含义可查看权限集介绍
 
| 参数名 | 类型 | 说明 |
|---|---|---|
| funcscope_category | __funcscope_category" data-v-90159960>object | 授权给开发者的权限集详情 |
 __funcscope_category" class="header-anchor" data-v-90159960>

### # Res.authorization_info.func_info(Array).funcscope_category Object Payload
 授权给开发者的权限集详情

| 参数名 | 类型 | 说明 |
|---|---|---|
| id | number | 权限集id |


## # 4. 枚举信息

### # Res.authorizer_info.register_type Enum
 小程序注册方式

| 枚举值 | 描述 |
|---|---|
| 0 | 普通方式注册 |
| 2 | 通过复用公众号创建小程序api注册 |
| 6 | 通过法人扫脸创建企业小程序api注册 |
| 13 | 通过创建试用小程序api注册 |
| 15 | 通过联盟控制台注册 |
| 16 | 通过创建个人小程序api注册 |
| 17 | 通过创建个人交易小程序api注册 |
| 19 | 通过试用小程序转正api注册 |
| 22 | 通过复用商户号创建企业小程序api注册 |
| 23 | 通过复用商户号转正api注册 |


### # Res.authorizer_info.account_status Enum
 账号状态，该字段小程序也返回

| 枚举值 | 描述 |
|---|---|
| 1 | 正常 |
| 14 | 已注销 |
| 16 | 已封禁 |
| 18 | 已告警 |
| 19 | 已冻结 |


### # Res.authorizer_info.service_type_info.id Enum
 类型id

| 枚举值 | 描述 |
|---|---|
| 0 | 订阅号 |
| 1 | 由历史老账号升级后的订阅号 |
| 2 | 服务号 |
| 0 | 普通小程序 |
| 12 | 试用小程序 |
| 4 | 小游戏 |
| 10 | 小商店 |
| 2或者3 | 门店小程序 |


### # Res.authorizer_info.verify_type_info.id Enum
 类型id

| 枚举值 | 描述 |
|---|---|
| -1 | 未认证 |
| 0 | 微信认证 |
| 1 | 新浪微博认证 |
| 3 | 已资质认证通过但还未通过名称认证 |
| 4 | 已资质认证通过、还未通过名称认证，但通过了新浪微博认证 |


### # Res.authorizer_info.channels_info.id Enum
 视频号类型

| 枚举值 | 描述 |
|---|---|
| 0 | 普通视频号 |


### # Res.authorizer_info.store_info.id Enum
 小店类型

| 枚举值 | 描述 |
|---|---|
| 0 | 普通小店 |


### # Res.authorizer_info.talent_info.id Enum
 带货助手类型

| 枚举值 | 描述 |
|---|---|
| 0 | 普通带货助手账号 |


### # Res.authorizer_info.supplier_info.id Enum
 供货商类型

| 枚举值 | 描述 |
|---|---|
| 0 | 普通供货商账号 |


## # 5. 注意事项
 注意： 公众号和小程序以及视频号的接口返回结果不一样。

## # 6. 代码示例

### # 6.1 公众号返回示例
 请求示例

```
{
  "component_appid": "appid_value",
  "authorizer_appid": "auth_appid_value"
}
```

返回示例

```
{
  "authorizer_info": {
    "nick_name": "微信SDK Demo Special",
    "head_img": "http://wx.qlogo.cn/mmopen/GPy",
    "service_type_info": {
      "id": 2
    },
    "verify_type_info": {
      "id": 0
    },
    "user_name": "gh_eb5e3a772040",
    "principal_name": "腾讯计算机系统有限公司",
    "business_info": {
      "open_store": 0,
      "open_scan": 0,
      "open_pay": 0,
      "open_card": 0,
      "open_shake": 0
    },
    "alias": "paytest01",
    "qrcode_url": "URL"
  },
  "authorization_info": {
    "authorizer_appid": "wxf8b4f85f3a794e77",
    "func_info": [
      {
        "funcscope_category": {
          "id": 1
        }
      },
      {
        "funcscope_category": {
          "id": 2
        }
      }
    ]
  }
}
```


### # 6.2 小程序返回示例
 请求示例

```
{
  "component_appid": "appid_value",
  "authorizer_appid": "auth_appid_value"
}
```

返回示例

```
{"authorizer_info":
     {"nick_name":"找呀找呀找盲盒",
      "head_img":"http:xxx",
      "service_type_info":{"id":0},
      "verify_type_info":{"id":-1},
      "user_name":"gh_3dacad47dc6b",
      "alias":"",
      "qrcode_url":"http:xxx",
      "business_info":{"open_pay":0,"open_shake":0,"open_scan":0,"open_card":0,"open_store":0},
      "idc":1,"principal_name":"个人","signature":"欢迎小伙伴一起参与盲盒游戏，集齐9款鞋卡，可以兑换大奖！",
      "MiniProgramInfo":{"network":{"RequestDomain":["https:xxx","https:xxxx","https:xxx"],"WsRequestDomain":[],"UploadDomain":[],"DownloadDomain":[],"BizDomain":[],"UDPDomain":[]},
                         "categories":[{"first":"工具","second":"效率"}],"visit_status":0}},
 "authorization_info":{"authorizer_appid":"wxf24d2dfc1a974128",
                       "authorizer_refresh_token":"xxxxxx",
                       "func_info":[{"funcscope_category":{"id":17}},
                                    {"funcscope_category":{"id":18}},
                                    {"funcscope_category":{"id":19}},{"funcscope_category":{"id":25}},
                                    {"funcscope_category":{"id":30}},
                                    {"funcscope_category":{"id":31}},
                                    {"funcscope_category":{"id":36}},{"funcscope_category":{"id":37}},{"funcscope_category":{"id":40}}
                                    ]}
 }
```


## # 7. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |


## # 8. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。
 接口变更日志（2条）
2026 年 03 月 17 日
新增  store_info 、 talent_info 、 supplier_info  字段

 2025 年 11 月 25 日
store_info 、 talent_info 、 supplier_info  字段新增，其  id  字段新增枚举值并描述
