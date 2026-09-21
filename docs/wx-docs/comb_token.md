
#################### token_preauthcode.md
# # 获取预授权码
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getPreAuthCode
 该接口用于获取预授权码（pre_auth_code）是第三方平台方实现授权托管的必备信息，每个预授权码有效期为 1800秒。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/component/api_create_preauthcode?access_token=ACCESS_TOKEN
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


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| pre_auth_code | string | 预授权码 |
| expires_in | number | 有效期，单位：秒 |


## # 4. 注意事项
 本接口无特殊注意事项

## # 5. 代码示例
 请求示例

```
{
  "component_appid": "appid_value"
}
```

返回示例

```
{
  "pre_auth_code": "Cx_Dk6qiBE0Dmx4EmlT3oRfArPvwSQ-oa3NL_fwHM7VI08r52wazoZX2Rhpz1dEw",
  "expires_in": 600
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |
| 40125 | invalid appsecret view more at http://t.cn/RAEkdVq | 无效的appsecret |
| 41001 | access_token missing | 缺少 access_token 参数 |
| 41004 | appsecret missing | 缺少 secret 参数 |
| 42001 | access_token expired | access_token 超时，请检查 access_token 的有效期，请参考基础支持 - 获取 access_token 中，对 access_token 的详细机制说明 |
| 45009 | reach max api daily quota limit | 调用超过天级别频率限制。可调用clear_quota接口恢复调用额度。 |
| 47001 | data format error | 解析 JSON/XML 内容错误;post 数据中参数缺失;检查修正后重试。 |
| 48001 | api unauthorized | api 功能未授权，请确认公众号已获得该接口，可以在公众平台官网 - 开发者中心页中查看接口权限 |
| 61004 | access clientip is not registered | 第三方平台出口IP未设置 |
| 61005 | component ticket is expired |  |
| 61006 | component ticket is invalid |  |
| 61011 | invalid component |  |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。


#################### token_getauthorizeraccesstoken.md
# # 获取授权账号调用令牌
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getAuthorizerAccessToken
 该接口用于获取授权账号的authorizer_access_token。authorizer_access_token 有效期为 2 小时，authorizer_access_token 失效时，可以使用 authorizer_refresh_token 获取新的 authorizer_access_token。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/component/api_authorizer_token?access_token=ACCESS_TOKEN
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
| authorizer_appid | string | 是 | 授权方 appid |
| authorizer_refresh_token | string | 是 | 刷新令牌，获取授权信息时得到 |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| authorizer_access_token | string | 授权方令牌 |
| expires_in | number | 有效期，单位：秒 |
| authorizer_refresh_token | string | 刷新令牌 |


## # 4. 注意事项
 authorizer_access_token 有效期为 2 小时，开发者需要缓存 authorizer_access_token，避免 API 调用触发每日限额。
 缓存方法可以参考：1000856

## # 5. 代码示例
 请求示例

```
{
  "component_appid": "appid_value",
  "authorizer_appid": "auth_appid_value",
  "authorizer_refresh_token": "refresh_token_value"
}
```

返回示例

```
{
  "authorizer_access_token": "some-access-token",
  "expires_in": 7200,
  "authorizer_refresh_token": "refresh_token_value"
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |
| 61014 | must use component token for component api | 原因是用错了token，使用了 access_token 或者 authorizer_access_token。应该使用component_access_token调用第三方平台的 API。 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。


#################### token_getauthorizerrefreshtoken.md
# # 获取刷新令牌
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getAuthorizerRefreshToken
 当用户在第三方平台授权页中完成授权流程后，第三方平台开发者可以在回调 URI 中通过 URL 参数获取授权码(authorization_code)。然后使用该接口可以换取公众号/小程序的刷新令牌（authorizer_refresh_token）。
 建议保存授权信息中的刷新令牌（authorizer_refresh_token）使用过程中如遇到问题，可在开放平台服务商专区发帖交流

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/component/api_query_auth?access_token=ACCESS_TOKEN
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
| authorization_code | string | 是 | 授权码, 会在授权成功时返回给第三方平台，详见第三方平台授权流程说明。该参数也可以通过平台推送的"授权变更通知"获取。 |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| authorization_info | object | 授权信息 |


### # Res.authorization_info Object Payload
 授权信息
 
| 参数名 | 类型 | 说明 |
|---|---|---|
| authorizer_appid | string | 授权的公众号或者小程序 appid |
| authorizer_access_token | string | 接口调用令牌（在授权的公众号/小程序具备 API 权限时，才有此返回值） |
| expires_in | number | authorizer_access_token 的有效期（在授权的公众号/小程序具备API权限时，才有此返回值），单位：秒 |
| authorizer_refresh_token | string | 刷新令牌（在授权的公众号具备API权限时，才有此返回值），刷新令牌主要用于第三方平台获取和刷新已授权用户的 authorizer_access_token。一旦丢失，只能让用户重新授权，才能再次拿到新的刷新令牌。用户重新授权后，之前的刷新令牌会失效 |
| func_info | " data-v-68f90c65>objarray | 授权给第三方平台的权限集id列表，权限集id代表的含义可查看权限集介绍 |
 " class="header-anchor" data-v-68f90c65>

### # Res.authorization_info.func_info(Array) Object Payload
 授权给第三方平台的权限集id列表，权限集id代表的含义可查看权限集介绍
 
| 参数名 | 类型 | 说明 |
|---|---|---|
| funcscope_category | __funcscope_category" data-v-68f90c65>object | 授权给开发者的权限集详情 |
 __funcscope_category" class="header-anchor" data-v-68f90c65>

### # Res.authorization_info.func_info(Array).funcscope_category Object Payload
 授权给开发者的权限集详情

| 参数名 | 类型 | 说明 |
|---|---|---|
| id | number | 权限集id |
| type | number | 权限集类型 |
| name | string | 权限集名称 |
| desc | string | 权限集描述 |


## # 4. 注意事项
 公众号/小程序可以自定义选择部分权限授权给第三方平台，因此第三方平台开发者需要通过该接口来获取公众号/小程序具体授权了哪些权限，而不是简单地认为自己声明的权限就是公众号/小程序授权的权限。
 刷新令牌 authorizer_refresh_token 有效期：参考生成说明-常见问题

## # 5. 代码示例
 请求示例

```
{
  "component_appid": "appid_value",
  "authorization_code": "auth_code_value"
}
```

返回示例

```
{
  "authorization_info": {
    "authorizer_appid": "wxf8b4f85f3a794e77",
    "authorizer_access_token": "QXjUqNqfYVH0yBE1iI_7vuN_9gQbpjfK7hYwJ3P7xOa88a89-Aga5x1NMYJyB8G2yKt1KCl0nPC3W9GJzw0Zzq_dBxc8pxIGUNi_bFes0qM",
    "expires_in": 7200,
    "authorizer_refresh_token": "dTo-YCXPL4llX-u1W1pPpnp8Hgm4wpJtlR6iV0doKdY",
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
      },
      {
        "funcscope_category": {
          "id": 3
        }
      }
    ]
  }
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。


#################### token_startpushticket.md
# # 启动票据推送服务
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：startPushTicket
 该 API 用于启动ticket推送服务。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/cgi-bin/component/api_start_push_ticket
```


### # 云调用
 本接口不支持云调用。

### # 第三方调用
 本接口仅支持第三方平台使用。

## # 2. 请求参数

### # 查询参数 Query String Parameters
 无

### # 请求体 Request Payload

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| component_appid | string | 是 | 平台型第三方平台的appid |
| component_secret | string | 是 | 平台型第三方平台的APPSECRET |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 错误码 |
| errmsg | string | 错误信息 |


## # 4. 注意事项
 本接口无特殊注意事项

## # 5. 代码示例
 请求示例

```
{
  "component_appid": "wxd0bfc95064b5bfc1",
  "component_secret": "28c223a63f7de0d5xxxxxxxxxxx"
}
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok"
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| 0 | ok或者in a normal state | ok是指从不正常变成正常 in a normal state是指本来就正常 |
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型调用。其他账号类型如无特殊说明，均不可调用。


#################### token_pre_auth_code_2.md
## # 预授权码
 预授权码（pre_auth_code）是第三方平台方实现授权托管的必备信息，每个预授权码有效期为 1800秒。需要先获取令牌才能调用。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

### # 请求地址

```
POST https://api.weixin.qq.com/cgi-bin/component/api_create_preauthcode?component_access_token=COMPONENT_ACCESS_TOKEN
```


### # 请求参数说明

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| component_access_token | string | 是 | 第三方平台component_access_token，不是authorizer_access_token |
| component_appid | string | 是 | 第三方平台 appid |

POST 数据示例：

```
{
  "component_appid": "appid_value" 
}
```


### # 结果参数说明

| 参数 | 类型 | 说明 |
|---|---|---|
| pre_auth_code | string | 预授权码 |
| expires_in | number | 有效期，单位：秒 |

返回结果示例：

```
{
  "pre_auth_code": "Cx_Dk6qiBE0Dmx4EmlT3oRfArPvwSQ-oa3NL_fwHM7VI08r52wazoZX2Rhpz1dEw",
  "expires_in": 600
}
```


### # 返回码说明

| 错误码 | 英文描述 | 中文描述 |
|---|---|---|
| 61004 | access clientip is not registered |  |
| 61005 | component ticket is expired |  |
| 41004 | appsecret missing | 缺少 secret 参数 |
| 40125 | invalid appsecret | 无效的appsecret |
| 61006 | component ticket is invalid |  |
| 61011 | invalid component |  |
| 45009 | reach max api daily quota limit | 接口调用超过限制 |
| 47001 | data format error | 解析 JSON/XML 内容错误 |
| 40001 | invalid credential, access_token is invalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 48001 | api unauthorized | api 功能未授权，请确认公众号/小程序已获得该接口，可以在公众平台官网 - 开发者中心页中查看接口权限 |
| 42001 | access_token expired | access_token 超时，请检查 access_token 的有效期，请参考基础支持 - 获取 access_token 中，对 access_token 的详细机制说明 |
| 40001 | invalid credential, access_token is invalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 41001 | access_token missing | 缺少 access_token 参数 |
| 61004 | access clientip is not registered |  |
| 40013 | invalid appid | 不合法的 AppID ，请开发者检查 AppID 的正确性，避免异常字符，注意大小写 |
| 61011 | invalid component |  |
| 48001 | api unauthorized | api 功能未授权，请确认公众号已获得该接口，可以在公众平台官网 - 开发者中心页中查看接口权限 |
| 其他错误码 |  | 请查看全局错误码 |



#################### token_api_query_auth_2.md
微信开放文档

(function () {
  'use strict';

  const SCRIPT_URLs = [
      'https://dldir1.qq.com/WechatWebDev/devPlatform/px.min.js',
      'https://dev.weixin.qq.com/platform-console/proxy/assets/tel/px.min.js',
  ];
  const param = {
      maskMode: 'no-mask', // 隐私策略, all-mask 或 no-mask, 详见：https://dev.weixin.qq.com/docs/analysis/sdk/docs.html
      recordCanvas: false,  // 若要采集canvas, 设为true
      projectId: 'wxef34f91ddab0c534-0HLdQNKAk-dzsFsA', // 项目 ID，需替换为体验分析项目 ID
      iframe: false, // 是否采集 iframe 页面
      console: true, // 是否采集 console 输出的错误日志
      network: true, // 是否采集网络错误
  };
  function loadScript(url) {
      return new Promise((resolve, reject) => {
          const scriptEle = document.createElement('script');
          scriptEle.type = 'text/javascript';
          scriptEle.async = true;
          scriptEle.src = url;
          scriptEle.onload = () => {
              resolve(url);
          };
          scriptEle.onerror = () => {
              reject(new Error('Script load error'));
          };
          document.head.appendChild(scriptEle);
      });
  }
  async function main() {
      try {
          sessionStorage.setItem('wxobs_start_timestamp', String(Date.now()));
          const fastestUrl = await Promise.race(SCRIPT_URLs.map(url => loadScript(url)));
          window.__startPX && window.__startPX(param);
      }
      catch (error) {
          console.error('Error loading scripts:', error);
      }
  }
  main();

})();

  小程序

  小游戏

  公众号

  服务号

  开放平台

  企业微信

  微信支付

  视频号

  微信小店

  智能对话

  腾讯小微

  教育平台

  社区

  学堂

 取消
  查看更多

 当前暂无结果，查看其它业务相关内容 >

  页面不存在或已失效
请点击 返回首页

#################### token_api_authorizer_token_2.md
## # 获取/刷新接口调用令牌
 在公众号/小程序接口调用令牌（authorizer_access_token）失效时，可以使用刷新令牌（authorizer_refresh_token）获取新的接口调用令牌。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。
 注意：
authorizer_access_token 有效期为 2 小时，开发者需要缓存 authorizer_access_token，避免获取/刷新接口调用令牌的 API 调用触发每日限额。缓存方法可以参考：https://developers.weixin.qq.com/doc/offiaccount/Basic_Information/Get_access_token.html

### # 请求地址

```
POST https://api.weixin.qq.com/cgi-bin/component/api_authorizer_token?component_access_token=COMPONENT_ACCESS_TOKEN
```


### # 请求参数说明

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| component_access_token | string | 是 | 第三方平台component_access_token |
| component_appid | string | 是 | 第三方平台 appid |
| authorizer_appid | string | 是 | 授权方 appid |
| authorizer_refresh_token | string | 是 | 刷新令牌，获取授权信息时得到 |

POST 数据示例：

```
{
  "component_appid": "appid_value",
  "authorizer_appid": "auth_appid_value",
  "authorizer_refresh_token": "refresh_token_value"
}
```


### # 结果参数说明

| 参数 | 类型 | 说明 |
|---|---|---|
| authorizer_access_token | string | 授权方令牌 |
| expires_in | nubmer 有效期，单位：秒 |  |
| authorizer_refresh_token | string | 刷新令牌 |

返回结果示例：

```
{
  "authorizer_access_token": "some-access-token",
  "expires_in": 7200,
  "authorizer_refresh_token": "refresh_token_value"
}
```


| 错误码 | 英文描述 | 中文描述 |
|---|---|---|
| 0 | ok | 成功 |
| 其他错误码 |  | 请查看全局错误码 |


