====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/en/openApi/OpenApiDoc/authorization-management/getAuthorizerInfo.html
====================================================================================================
# # Get Authorized Account Details
  Debugging tools
 初始值Hattori API。

## # Interface Dxplaination

### # Interface name
 getAuthorizerInfo

### # Functional description
 this API Used to obtain the basic information of the authorized party, including avatar, nickname, account type, authentication type, original ID and other information. If you encounter problems during use, you canOpen Platform Service Provider ZonePosting exchanges

### # Note
 Note: Official Account message template Not the same as the interface of the Mini Program.

## # Calling mode

### # HTTPS call

```
POST https://api.weixin.qq.com/cgi-bin/component/api_get_authorizer_info?access_token=ACCESS_TOKEN
```


### # Request parameters

| attribute | type | Required | Introductions |
|---|---|---|---|
| access_Token | string | yes | Interface invokes the certificate, which is URL Parameters, non Body Parameters. usecomponent_access_Token |
| component_appid | string | yes | Third-Party Platforms appid |
| authorizer_appid | string | yes | Authorized appid of Official Account message template or Mini Program |


### # Return parameters

|  | attribute | type | Introductions |
|---|---|---|---|
|  | authorizer_info | object | Authorized account information |
|  |  | attribute | type | Introductions |
|  | nick_name | string | nickname |
|  | head_img | string | avatar |
|  | service_type_info | object | Official Account message template/Mini Program type |
|  |  | attribute | type | Introductions |
|  | id | number | Type id |
|  | name | string | Type explaination |

  verify_type_info object Official Account message template/Mini Program Authentication Type

|  | attribute | type | Introductions |
|---|---|---|---|
|  | id | number | Type id |
|  | name | string | Type explaination |

  user_name string Original ID
  alias string Official Account message template The WeChat ID set may be empty
  qrcode_url string QR Code Picture URL
  business_info object To understand the opening status of the function (0 means not opened, 1 means opened)

|  | attribute | type | Introductions |
|---|---|---|---|
|  | open_pay | number | Whether to open WeChat payment function |
|  | open_shake | number | Whether to open WeChat shake function |
|  | open_scan | number | Whether to open WeChat sweep product function |
|  | open_card | number | Open WeChat card voucher function |
|  | open_store | number | Open WeChat store function |

  idc number Abandoned parameter
  principal_name 初始值 Principal name
  signature string Small Program Account Introduction
  MiniProgramInfo object Mini Program configuration information, based on this field to determine whether or not Mini Program type authorization

|  | attribute | type | Introductions |
|---|---|---|---|
|  | network | object | Legal domain name information configured by the Mini Program |
|  |  | attribute | type | Introductions |
|  | RequestDomain | array | Request a Domain Name |
|  | WsRequestDomain | array | socket domain name |
|  | UploadDomain | array | uploadFile |
|  | DownloadDomain | array | downloadFile |
|  | UDPDomain | 初始值string> | UDP Legal Domains |
|  | TCPDomain | array | Tcp legal domain name |

  categories array Category information for Mini Program configuration

|  | attribute | type | Introductions |
|---|---|---|---|
|  | first | string | First class category |
|  | second | string | Secondary category |

  visit_status number Abandoned parameter

  register_type number Small Program Registration
  account_status number Account status, the field Mini Program also returns
  basic_config object Basic configuration information

|  | attribute | type | Introductions |
|---|---|---|---|
|  | is_phone_configured | boolean | Is the phone number already attached? |
|  | is_email_configured | boolean | Whether already bound mailbox, not bound mailbox account can not log on WeChat Official Platform |


  authorization_info object Authorization Information

|  | attribute | type | Introductions |
|---|---|---|---|
|  | authorizer_appid | string | Authorized Official Account message template Or Mini programs. appid |
|  | authorizer_初始值_Token | string | Refresh Token (this return value is only available if the authorized Official Account message template has API permissions), which is primarily used by a third-party platform to obtain and refresh an authorized user's authorizer_access_token。 Once lost, the user can only be re-authorized to get a new refresh token again. When the user reauthorizes, the previous refresh token will expire |
|  | func_info | array | A list of permission set IDs authorized to third-party platforms. The meaning of permission set IDs can be viewedIntroduction to Permission Sets |
|  |  | attribute | type | Introductions |
|  | funcscope_category | object | Details of the permission set granted to the developer |
|  |  | attribute | type | Introductions |
|  | id | number | Permission set id |
|  | type | number | Permission Set Type |
|  | name | string | Permission Set Name |
|  | desc | string | Permission Set Description |


## # Other Notes

### # Official Account message template type

| type | Introductions |
|---|---|
| 0 | Subscription number |
| 1 | The subscription number after the upgrade from the old account |
| 2 | Service number |


### # Official Account message template Type of certification

| type | Introductions |
|---|---|
| -1 | Not certified |
| 初始值 | WeChat authentication |
| 1 | Sina Weibo Authentication |
| 2 | Tencent microblogging certification |
| 3 | Certified but not yet certified by name |
| 4 | Has passed the qualification certification, has not passed the name certification, but passed the Sina Weibo certification |
| 5 | Has passed the qualification certification, has not passed the name certification, but passed the Tencent micro-blog certification |


### # Account Status

| type | Introductions |
|---|---|
| 1 | normal |
| 14 | Cancelled |
| 16 | Banned |
| 18 | Alert. |
| 19 | Frozen |


### # Mini Program type

| type | Introductions |
|---|---|
| 0 | Ordinary Mini Program |
| 12 | Trial Mini Program |
| 4 | Games |
| 10 | Mini Store |
| Two or three. | Store Mini Program |


### # Mini Program Registration Type

| type | Introductions |
|---|---|
| 0 | Ordinary Way Registration |
| 2 | Create Mini Program api registration by reusing the Official Account message template |
| 6 | Create an Enterprise Mini Program API Registration by Legal Person Sweep Face |
| 13 | Register by creating a trial Mini Program api |
| 15 | Register through the Affiliate Console |
| 16 | Register by creating a personal Mini Program api |
| 17 | Register by creating a personal trading Mini Program api |
| 19 | Change api registration through trial Mini Program |
| 22 | Creating an Enterprise Mini Program API Registration by Reusing Merchant Numbers |
| 23 | Convert api registration through reusing merchant number |


### # Mini Program Authentication Type

| type | Introductions |
|---|---|
| -1 | Not certified |
| 0 | WeChat authentication |


## # 初始值
 Example Dxplaination: Official Account message template Return Example

### # Sample Request Data

```
{
  "component_appid": "appid_value" ,
  "authorizer_appid": "auth_appid_value"
}
```


### # Return Data Example

```
{
  "authorizer_info": {
    "nick_name": " WeChat SDK Demo Special",
    "head_img": "http://wx.qlogo.cn/初始值/GPy",
    "service_type_info": {
      初始值: 2
    },
    "verify_type_info": {
      "id": 0
    },
    "user_name": "gh_eb5e3a772040",
    "principal_name": "Tencent Computer Systems Company Limited ",
    "business_info": {
      "open_store": 0,
      "open_初始值": 0,
      "open_pay " : 0,
      "open_card": 0,
      "open_shake": 0
    },
    "alias": "paytest01",
    "qrcode_url": "URL",
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

Example Dxplaination: Small program return example

### # Sample Request Data

```
{
  "component_appid": "appid_value" ,
  "authorizer_appid": "auth_appid_value"
}
```


### # Return Data Example

```
{"authorizer_info":
     {"nick_Name ":" Looking and looking and looking for the blind box, "
      "head_img":"http:xxx",
      " service_type_info":{"id":0},
      "verify_type_info":{"id":-1},
      "user_name":"gh_3dacad47dc6b",
      "alias":"",
      qrcode_url":"http:xxx"
      "business_info":{"open_pay":0,"open_shake":0,"open_scan":0,"open_card":0,"open_store":0},
      "idc": "main"_Name ":" Personal, "" Signature ":" Welcome partners to participate in the blind box game, collect 9 shoe cards, you can redeem the grand prize! " ,
      "MiniProgramInfo":{"network":{"RequestDomain":["https:xxx","https:xxxx","https:xxx"],"WsRequestDomain":[],"UploadDomain":[],"DownloadDomain":[],"BizDomain":[],"UDPDomain":[]},
                         "categories":[{"first": "tools," "second": "efficiency"}],"visit_status:
 "authorization_info":{"authorizer_appid":"wxf24d2dfc1a974128"
                       "authorizer_refresh_token":"xxx"
                       "func_info":[{"funcscope_category":{"id":17}},
                                    {"funcscope_category":{"id":18}},
                                    {"funcscope_category":{"id":19}},{"funcscope_category":{"id":25}},
                                    {"funcscope_category":{"id":30}},
                                    {"funcscope_category":{"id":31}},
                                    {"funcscope_category":{"id":36}},{"funcscope_category":{"id":37}},{"funcscope_category":{"id":40}},
                                    ]}
 }
```


### # Error code

| Error code | Error code | Solutions |
|---|---|---|
| -1 | system error | The system is busy, please wait for the developer to try again |
| 40001 | invalid credential access_Token isinvalid or not latest | Obtain access_Token time AppSecret Error, or access_Token Invalid. Please take the developer more seriously. AppSecret Of the correctness, or to see if you are working for the appropriate Official Account message template Call interface |
| 40013 | invalid appid | Illegal AppID , ask the developer to check AppID The correctness of the, avoid unusual characters, pay attention to the case |
| 0 | ok | ok |


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
