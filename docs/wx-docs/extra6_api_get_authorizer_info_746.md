====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/ThirdParty/token/api_get_authorizer_info.html
====================================================================================================
此文档已经过期

## # 获取授权账号详情
 该 API 用于获取授权方的基本信息，包括头像、昵称、账号类型、认证类型、微信号、原始ID和二维码图片URL。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。
 注意：
公众号和小程序的接口返回结果不一样。

### # 请求地址

```
POST https://api.weixin.qq.com/cgi-bin/component/api_get_authorizer_info?component_access_token=COMPONENT_ACCESS_TOKEN
```


### # 请求参数说明

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| component_access_token | string | 是 | 第三方平台component_access_token，不是authorizer_access_token |
| component_appid | string | 是 | 第三方平台 appid |
| authorizer_appid | string | 是 | 授权方 appid |

POST 数据示例：

```
{
  "component_appid": "appid_value" ,
  "authorizer_appid": "auth_appid_value"
}
```


### # 返回参数说明（公众号）

| 参数 | 类型 | 说明 |
|---|---|---|
| authorization_info | object | 授权信息，详见authorization_info |
| authorizer_info | object | 详见公众号账号信息 |


#### # 公众号账号信息

| 参数 | 类型 | 说明 |
|---|---|---|
| nick_name | string | 昵称 |
| head_img | string | 头像 |
| service_type_info | object | 公众号类型 |
| verify_type_info | object | 公众号认证类型 |
| user_name | string | 原始 ID |
| principal_name | string | 主体名称 |
| alias | string | 公众号所设置的微信号，可能为空 |
| business_info | object | 用以了解功能的开通状况（0代表未开通，1代表已开通），详见business_info 说明 |
| qrcode_url | string | 二维码图片的 URL，开发者最好自行也进行保存 |
| account_status | number | 账号状态，该字段小程序也返回 |


##### # 公众号类型

| 类型 | 说明 |
|---|---|
| 0 | 订阅号 |
| 1 | 由历史老账号升级后的订阅号 |
| 2 | 服务号 |


##### # 公众号认证类型

| 类型 | 说明 |
|---|---|
| -1 | 未认证 |
| 0 | 微信认证 |
| 1 | 新浪微博认证 |
| 3 | 已资质认证通过但还未通过名称认证 |
| 4 | 已资质认证通过、还未通过名称认证，但通过了新浪微博认证 |


##### # 账号状态

| 类型 | 说明 |
|---|---|
| 1 | 正常 |
| 14 | 已注销 |
| 16 | 已封禁 |
| 18 | 已告警 |
| 19 | 已冻结 |

返回结果示例（公众号）：

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
    "qrcode_url": "URL",
"account_status":1
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


### # 返回参数说明（小程序）

| 参数 | 类型 | 说明 |
|---|---|---|
| authorizer_info | object | 详见小程序账号信息 |
| authorization_info | object | 授权信息，详见authorization_info |


#### # 小程序账号信息

| 参数 | 类型 | 说明 |
|---|---|---|
| nick_name | string | 昵称 |
| head_img | string | 头像 |
| service_type_info | object | 小程序类型 |
| verify_type_info | object | 小程序认证类型 |
| user_name | string | 原始 ID |
| principal_name | string | 主体名称 |
| signature | string | 账号介绍 |
| business_info | object | 用以了解功能的开通状况（0代表未开通，1代表已开通），详见business_info 说明 |
| qrcode_url | string | 二维码图片的 URL，开发者最好自行也进行保存 |
| account_status | number | 账号状态，该字段公众号也返回 |
| register_type | number | 小程序注册方式 |
| basic_config | object | 基础配置信息 |
| MiniProgramInfo | object | 小程序配置，根据这个字段判断是否为小程序类型授权 |


##### # basic_config结构体

| 参数 | 类型 | 说明 |
|---|---|---|
| is_phone_configured | bool | 是否已经绑定手机号 |
| is_email_configured | bool | 是否已经绑定邮箱，不绑定邮箱账号的不可登录微信公众平台 |


##### # 小程序类型

| 类型 | 说明 |
|---|---|
| 0 | 普通小程序 |
| 12 | 试用小程序 |
| 4 | 小游戏 |
| 10 | 小商店 |
| 2或者3 | 门店小程序 |


##### # 小程序注册类型

| 类型 | 说明 |
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


##### # 小程序认证类型

| 类型 | 说明 |
|---|---|
| -1 | 未认证 |
| 0 | 微信认证 |


##### # 小程序配置说明

| 参数 | 类型 | 说明 |
|---|---|---|
| network | object | 小程序配置的合法域名信息 |
| categories | object array | 小程序配置的类目信息 |

返回结果示例（小程序）：

```
{
  "authorizer_info": {
    "nick_name": "美妆饰品",
    "head_img": "http:\/\/wx.qlogo.cn\/mmopen\/jJSbu4Te5ibibv2mJFb9ho1JuAfW9tyic5NX0Vhia4GMv3RDAdh3gTia6ewrbqYpn65UTxl7nT56nuiaM0dFnKVUEE83n2yH5cQStb\/0",
    "service_type_info": {
      "id": 0
    },
    "verify_type_info": {
      "id": -1
    },
    "user_name": "gh_c43395cb652e",
    "alias": "",
    "qrcode_url": "http:\/\/mmbiz.qpic.cn\/mmbiz_jpg\/kPxpXe3ic7TBGOvHkK1rGplicjachD5iaLic75NthsCZcd2CYoqkJAo7YPqEndQcSyCDNGXic7F00yWdhOFZGmmhe6g\/0",
    "business_info": {
      "open_pay": 0,
      "open_shake": 0,
      "open_scan": 0,
      "open_card": 0,
      "open_store": 0
    },
    "idc": 1,
    "principal_name": "个人",
    "signature": "做美装，精美饰品等搭配教学",
    "MiniProgramInfo": {
      "network": {
        "RequestDomain": ["https:\/\/weixin.qq.com"],
        "WsRequestDomain": ["wss:\/\/weixin.qq.com"],
        "UploadDomain": ["https:\/\/weixin.qq.com"],
        "DownloadDomain": ["https:\/\/weixin.qq.com"],
        "BizDomain": [],
        "UDPDomain": [],
        "TCPDomain": [],
        "PrefetchDNSDomain": [],
        "NewRequestDomain": [],
        "NewWsRequestDomain": [],
        "NewUploadDomain": [],
        "NewDownloadDomain": [],
        "NewBizDomain": [],
        "NewUDPDomain": [],
        "NewTCPDomain": [],
        "NewPrefetchDNSDomain": []
      },
      "categories": [{
        "first": "生活服务",
        "second": "丽人服务"
      }, {
        "first": "旅游服务",
        "second": "旅游资讯"
      }, {
        "first": "物流服务",
        "second": "查件"
      }],
      "visit_status": 0
    },
    "register_type": 0,
    "account_status": 1,
    "basic_config": {
      "is_phone_configured": true,
      "is_email_configured": true
    }
  },
  "authorization_info": {
    "authorizer_appid": "wx326eecacf7370d4e",
    "authorizer_refresh_token": "refreshtoken@@@RU0Sgi7bD6apS7frS9gj8Sbws7OoDejK9Z-cm0EnCzg",
    "func_info": [{
      "funcscope_category": {
        "id": 3
      },
      "confirm_info": {
        "need_confirm": 0,
        "already_confirm": 0,
        "can_confirm": 0
      }
    }, {
      "funcscope_category": {
        "id": 7
      }
    }, {
      "funcscope_category": {
        "id": 17
      }
    }, {
      "funcscope_category": {
        "id": 18
      },
      "confirm_info": {
        "need_confirm": 0,
        "already_confirm": 0,
        "can_confirm": 0
      }
    }, {
      "funcscope_category": {
        "id": 19
      }
    }, {
      "funcscope_category": {
        "id": 30
      },
      "confirm_info": {
        "need_confirm": 0,
        "already_confirm": 0,
        "can_confirm": 0
      }
    }, {
      "funcscope_category": {
        "id": 115
      }
    }]
  }
}
```


#### # authorization_info

| 参数 | 类型 | 说明 |
|---|---|---|
| authorization_appid | string | 授权方 appid |
| func_info | object | 授权给开发者的权限集列表 |


##### # business_info 说明

| 功能 | 说明 |
|---|---|
| open_store | 是否开通微信门店功能 |
| open_scan | 是否开通微信扫商品功能 |
| open_pay | 是否开通微信支付功能 |
| open_card | 是否开通微信卡券功能 |
| open_shake | 是否开通微信摇一摇功能 |


### # 返回码说明

| 错误码 | 英文描述 | 中文描述 |
|---|---|---|
| 0 | ok | 成功 |
| 其他错误码 |  | 请查看全局错误码 |


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
