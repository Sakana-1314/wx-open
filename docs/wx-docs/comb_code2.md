
#################### code_getauditstatus.md
# # 查询审核单状态
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getAuditStatus
 通过接口提交代码审核后，调用本接口可以查询指定发布审核单的审核状态。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/wxa/get_auditstatus?access_token=ACCESS_TOKEN
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
| auditid | number | 是 | 提交审核时获得的审核 id |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 返回码 |
| errmsg | string | 错误信息 |
| status | number | 审核状态 |
| reason | string | 当 status = 1 时，返回的拒绝原因; status = 4 时，返回的延后原因 |
| screenshot | string | 当 status = 1 时，会返回审核失败的小程序截图示例。用竖线分隔的 media_id 的列表，可通过获取永久素材接口拉取截图内容 |


## # 4. 注意事项

# # 其他说明
 请注意调用如下接口时需要使用第三方平台接口调用令牌authorizer_access_token
 获取永久素材接口

## # 审核状态说明

| 状态值 | 说明 |
|---|---|
| 0 | 审核成功 |
| 1 | 审核被拒绝 |
| 2 | 审核中 |
| 3 | 已撤回 |
| 4 | 审核延后 |


## # 5. 代码示例
 请求示例

```
{
  "auditid": 1234567
}
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "status": 1,
  "reason": "帐号信息不合规范",
  "screenshot": "xxx|yyy|zzz"
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 85012 | invalid audit id | 无效的审核 id |
| 86000 | should be called only from third party | 不是由第三方代小程序进行调用 |
| 86001 | component experience version not exists | 不存在第三方的已经提交的代码 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。


#################### code_getlatestauditstatus.md
# # 查询最新一次审核单状态
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getLatestAuditStatus
 调用本接口可以查询最新一次提审单的审核状态。使用过程中如遇到问题，可在开放平台服务商专区发帖交流

## # 1. 调用方式

### # HTTPS 调用

```
GET https://api.weixin.qq.com/wxa/get_latest_auditstatus?access_token=ACCESS_TOKEN
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
| errcode | number | 错误码 |
| errmsg | string | 错误信息 |
| auditid | number | 最新的审核id |
| status | number | 审核状态 |
| reason | string | 当审核被拒绝时，返回的拒绝原因 |
| screenshot | string | 当审核被拒绝时，会返回审核失败的小程序截图示例。用 竖线I 分隔的 media_id 的列表，可通过获取永久素材接口拉取截图内容 |
| user_version | string | 审核版本 |
| user_desc | string | 版本描述 |
| submit_audit_time | number | 时间戳，提交审核的时间 |


## # 4. 注意事项

# # 其他说明
 请注意调用如下接口时需要使用第三方平台接口调用令牌authorizer_access_token
 获取永久素材接口

## # 审核状态说明

| 状态值 | 说明 |
|---|---|
| 0 | 审核成功 |
| 1 | 审核被拒绝 |
| 2 | 审核中 |
| 3 | 已撤回 |
| 4 | 审核延后 |


## # 5. 代码示例
 请求示例

```
GET https://api.weixin.qq.com/wxa/get_latest_auditstatus?access_token=ACCESS_TOKEN
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "auditid": 1234567,
  "status": 1,
  "reason": "帐号信息不合规范",
  "ScreenShot": "xx|yy|zz",
  "user_version": "V1.5",
  "user_desc": "test",
  "submit_audit_time": 1640763673
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。


#################### code_undoaudit.md
# # 撤回代码审核
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：undoAudit
 调用本接口可以撤回当前的代码审核单。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
GET https://api.weixin.qq.com/wxa/undocodeaudit?access_token=ACCESS_TOKEN
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


## # 4. 注意事项
 单个账号每天审核撤回次数最多不超过 5 次（每天的额度从0点开始生效），一个月不超过 10 次。

## # 5. 代码示例
 请求示例

```
GET https://api.weixin.qq.com/wxa/undocodeaudit?access_token=ACCESS_TOKEN
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
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 87013 | no quota to undo code | 撤回次数达到上限（每天5次，每个月 10 次） |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。


#################### code_release.md
# # 发布已通过审核的小程序
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：release
 调用本接口可以发布最后一个审核通过的小程序代码版本。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/wxa/release?access_token=ACCESS_TOKEN
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


## # 4. 注意事项
 注意，post的data为空，不等于不需要传data，否则会报错【errcode: 44002 "errmsg": "empty post data"】

## # 5. 代码示例
 请求示例

```
{}
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
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 40014 | invalid access_token | 不合法的 access_token ，请开发者认真比对 access_token 的有效性（如是否过期），或查看是否正在为恰当的公众号调用接口 |
| 85019 | no version is under auditing | 没有审核版本 |
| 85020 | status not allowed | 审核状态未满足发布 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。


#################### code_revertcoderelease.md
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


#################### code_getversioninfo.md
# # 查询小程序版本信息
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：getVersionInfo
 调用本接口可以查询小程序的体验版和线上版本信息。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

 说明：如果需要查询最新提交的代码审核中版本信息可通过getLatestAuditStatus接口获取。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/wxa/getversioninfo?access_token=ACCESS_TOKEN
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
| exp_info | object | 体验版信息 |
| release_info | object | 线上版信息 |


### # Res.exp_info Object Payload
 体验版信息

| 参数名 | 类型 | 说明 |
|---|---|---|
| exp_time | number | 提交体验版的时间 |
| exp_version | string | 体验版版本信息 |
| exp_desc | string | 体验版版本描述 |


### # Res.release_info Object Payload
 线上版信息

| 参数名 | 类型 | 说明 |
|---|---|---|
| release_time | number | 发布线上版的时间 |
| release_version | string | 线上版版本信息 |
| release_desc | string | 线上版本描述 |


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
  "exp_info": {
    "exp_time": 1640762988,
    "exp_version": "V1.0",
    "exp_desc": "test"
  },
  "release_info": {
    "release_time": 1640763902,
    "release_version": "V1.5",
    "release_desc": "test"
  }
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 44002 | empty post data | POST 的数据包为空 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。


#################### code_speedupcodeaudit.md
# # 加急代码审核
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：speedupCodeAudit
 有加急次数的第三方可以通过该接口，对已经提审的小程序进行加急操作，加急后的小程序预计2-12小时内审完。但是若代码中包含较复杂逻辑或其他特殊情况，可能会导致审核时间延长。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/wxa/speedupaudit?access_token=ACCESS_TOKEN
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
| auditid | number | 是 | 审核单ID |


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
  "auditid": 12345
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
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 89401 | 系统不稳定，请稍后再试，如多次失败请通过社区反馈 | 系统不稳定，请稍后再试，如多次失败请通过社区反馈 |
| 89402 | 该小程序不在待审核队列，请检查是否已提交审核或已审完 | 该小程序不在待审核队列，请检查是否已提交审核或已审完 |
| 89403 | 本单属于平台不支持加急种类，请等待正常审核流程 | 本单属于平台不支持加急种类，请等待正常审核流程 |
| 89404 | 本单已加速成功，请勿重复提交 | 本单已加速成功，请勿重复提交 |
| 89405 | 本月加急额度已用完，请提高提审质量以获取更多额度 | 本月加急额度已用完，请提高提审质量以获取更多额度 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。


#################### code_setcodeauditquota.md
# # 查询服务商审核额度
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：setCodeAuditQuota
 服务商可以调用该接口，查询当月平台分配的提审限额和剩余可提审次数，以及当月分配的审核加急次数和剩余加急次数。（所有旗下小程序共用该额度） 使用过程中如遇到问题，可在开放平台服务商专区发帖交流。
 若额度不足，需要申请提额，查看第三方小程序提审quota。

## # 1. 调用方式

### # HTTPS 调用

```
GET https://api.weixin.qq.com/wxa/queryquota?access_token=ACCESS_TOKEN
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
| rest | number | quota剩余值 |
| limit | number | 当月分配quota |
| speedup_rest | number | 剩余加急次数 |
| speedup_limit | number | 当月分配加急次数 |


## # 4. 注意事项
 本接口无特殊注意事项

## # 5. 代码示例
 请求示例

```
GET https://api.weixin.qq.com/wxa/queryquota?access_token=ACCESS_TOKEN
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "rest": 0,
  "limit": 0,
  "speedup_rest": 0,
  "speedup_limit": 0
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 0 | ok | ok |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

