====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_submitaudit.html
====================================================================================================
# # 提交代码审核
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：submitAudit
 在调用commit接口为小程序上传代码后，可以调用本接口，将上传的代码提交审核。
 为了提高提交代码审核成功率，开发者可先阅读《提交代码审核的前置检查项》说明
使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/wxa/submit_audit?access_token=ACCESS_TOKEN
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
| item_list | " data-v-6f63fefa>objarray | 是 | 审核项列表（选填，至多填写 5 项）；类目是必填的，且要填写已经在小程序配置好的类目。 |
| feedback_info | string | 否 | 反馈内容，至多 200 字 |
| feedback_stuff | string | 否 | 用 | 分割的 media_id 列表，至多 5 张图片, 可以通过新增临时素材接口上传而得到 |
| version_desc | string | 否 | 小程序版本说明和功能解释 |
| preview_info | object | 否 | 预览信息（小程序页面截图和操作录屏） |
| ugc_declare | object | 否 | 用户生成内容场景（UGC）信息安全声明 |
| privacy_api_not_use | boolean | 否 | 用于声明是否不使用“代码中检测出但是未配置的隐私相关接口” |
| order_path | string | 否 | 订单中心path |
 " class="header-anchor" data-v-6f63fefa>

### # Body.item_list(Array) Object Payload
 审核项列表（选填，至多填写 5 项）；类目是必填的，且要填写已经在小程序配置好的类目。

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| address | string | 否 | 小程序的页面，可通过"获取小程序的页面列表getCodePage"接口获得 |
| tag | string | 否 | 小程序的标签，用空格分隔，标签至多 10 个，标签长度至多 20 |
| first_class | string | 是 | 一级类目名称，可通过"getAllCategoryName"接口获取 |
| second_class | string | 是 | 二级类目名称，可通过"getAllCategoryName"接口获取 |
| third_class | string | 否 | 三级类目名称，可通过"getAllCategoryName"接口获取 |
| title | string | 否 | 小程序页面的标题,标题长度至多 32 |
| first_id | number | 是 | 一级类目id，可通过"getAllCategoryName"接口获取 |
| second_id | number | 是 | 二级类目id，可通过"getAllCategoryName"接口获取 |
| third_id | number | 否 | 三级类目id，可通过"getAllCategoryName"接口获取 |


### # Body.preview_info Object Payload
 预览信息（小程序页面截图和操作录屏）

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| video_id_list | array | 是 | 录屏mediaid列表，可以通过上传提审素材获得 |
| pic_id_list | array | 是 | 截屏mediaid列表，可以通过上传提审素材接口获得 |


### # Body.ugc_declare Object Payload
 用户生成内容场景（UGC）信息安全声明

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| scene | - | 否 | UGC场景 0,不涉及用户生成内容, 1.用户资料,2.图片,3.视频,4.文本,5音频, 可多选,当scene填0时无需填写下列字段 |
| method | - | 否 | 内容安全机制 1.使用平台建议的内容安全API,2.使用其他的内容审核产品,3.通过人工审核把关,4.未做内容审核把关 |
| other_scene_desc | string | 否 | 当scene选其他时的说明,不超时256字 |
| has_audit_team | number | 否 | 是否有审核团队, 0.无,1.有,默认0 |
| audit_desc | string | 否 | 说明当前对UGC内容的审核机制,不超过256字 |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 返回码 |
| errmsg | string | 错误信息 |
| auditid | number | 审核编号 |


## # 4. 注意事项
 如果接口中涉及地理位置相关隐私接口，需要在提交代码的接口的ext.json参数中配置requiredPrivateInfos
 上传代码后，需要等检测任务结束后方可提交代码审核，否则提交代码审核时会出现61039报错
 开发者可以通过getCodePrivacyInfo接口获取检测结果，确认检测任务结束后再提交代码审核
 不要将上传代码和提交代码审核两个接口一起重试，否则每次上传代码后的检测任务未结束又发起新的检测任务，依旧会出现61039报错。
 如果代码里使用了地理位置相关接口，但是又尚未在ext_json参数中配置requiredPrivateInfos。那么提交代码审核时会出现61040报错。
 如果代码里使用了地理位置相关接口，但是该小程序appid又未获得对应地理位置api的权限，那么提交代码审核时会出现61040报错。
 如果检测出了代码包含了隐私相关接口，但是开发者确认并未使用，则可以通过privacy_api_not_use进行声明
 境外主体小程序需要补充「用户隐私保护指引」中「存储地区」的相关信息，否则小程序审核会被驳回。可查看公告
，调用接口setPrivacySetting设置小程序用户隐私保护指引配置store_region存储地区

# # 其他说明
 临时素材 mediaid 通过调用“临时素材管理接口”获取
 请注意调用如下接口时需要使用第三方平台接口调用令牌authorizer_access_token
 新增临时素材
 审核不通过截图可通过获取永久素材接口拉取截图内容

## # 补充说明
 只有上个版本被驳回，才能使用 feedback_info、feedback_stuff 这两个字段，否则忽略处理。
 当小程序第一次提交审核且类目包含社交-社区/论坛、社交-笔记、社交-问答其中之一时需填写 ugc_declare
 可另外调用上传提审素材接口，将小程序页面截图和操作录屏上传，提审时带上相关参数，可以帮助审核人员判断。

## # 代码审核结果推送
 当小程序有审核结果后，微信服务器会向第三方平台方的消息与事件接收 URL（创建第三方平台时填写）以 POST 的方式推送相关通知，详情见消息推送文档。
 接收 POST 请求后，只需直接返回字符串 success。
 除了消息通知之外，第三方平台也可通过接口查询指定版本的审核状态、查询最新一次提交的审核状态

### # 字段说明

| 参数 | 类型 | 说明 |
|---|---|---|
| ToUserName | String | 小程序的原始 ID |
| FromUserName | String | 发送方账号（一个 OpenID，此时发送方是系统账号） |
| CreateTime | Number | 消息创建时间 （整型），时间戳 |
| MsgType | String | 消息类型 event |
| Event | String | 事件类型 |
| SuccTime | Number | 审核成功时的时间戳 |
| FailTime | Number | 审核不通过的时间戳 |
| DelayTime | Number | 审核延后时的时间戳 |
| Reason | String | 审核不通过的原因 |
| ScreenShot | String | 审核不通过的截图示例。用 | 分隔的 media_id 的列表，可通过获取永久素材接口拉取截图内容 |


#### # 事件类型

| 事件类型 | 说明 |
|---|---|
| weapp_audit_success | 审核通过 |
| weapp_audit_fail | 审核不通过 |
| weapp_audit_delay | 审核延后 |

推送内容解密后的示例：

#### # 审核通过

```
<xml>
  <ToUserName><![CDATA[gh_fb9688c2a4b2]]></ToUserName>
  <FromUserName><![CDATA[od1P50M-fNQI5Gcq-trm4a7apsU8]]></FromUserName>
  <CreateTime>1488856741</CreateTime>
  <MsgType><![CDATA[event]]></MsgType>
  <Event><![CDATA[weapp_audit_success]]></Event>
  <SuccTime>1488856741</SuccTime>
</xml>
```


#### # 审核不通过

```
<xml>
  <ToUserName><![CDATA[gh_fb9688c2a4b2]]></ToUserName>
  <FromUserName><![CDATA[od1P50M-fNQI5Gcq-trm4a7apsU8]]></FromUserName>
  <CreateTime>1488856591</CreateTime>
  <MsgType><![CDATA[event]]></MsgType>
  <Event><![CDATA[weapp_audit_fail]]></Event>
  <Reason><![CDATA[1:账号信息不符合规范:<br>(1):包含色情因素<br>2:服务类目"金融业-保险_"与你提交代码审核时设置的功能页面内容不一致:<br>(1):功能页面设置的部分标签不属于所选的服务类目范围。<br>(2):功能页面设置的部分标签与该页面内容不相关。<br>]]></Reason>
  <FailTime>1488856591</FailTime>
  <ScreenShot>xxx|yyy|zzz</ScreenShot>
</xml>
```


#### # 审核延后

```
<xml>
  <ToUserName><![CDATA[gh_fb9688c2a4b2]]></ToUserName>
  <FromUserName><![CDATA[od1P50M-fNQI5Gcq-trm4a7apsU8]]></FromUserName>
  <CreateTime>1488856591</CreateTime>
  <MsgType><![CDATA[event]]></MsgType>
  <Event><![CDATA[weapp_audit_delay]]></Event>
  <Reason><![CDATA[为了更好的服务小程序，您的服务商正在进行提审系统的优化，可能会导致审核时效的增长，请耐心等待]]></Reason>
  <DelayTime>1488856591</DelayTime>
</xml>
```


## # 5. 代码示例
 请求示例

```
{
	"item_list": [
	{
		"address":"index",
		"tag":"学习 生活",
		"first_class": "文娱",
		"second_class": "资讯",
		"first_id":1,
		"second_id":2,
		"title": "首页"
	}
	{
		"address":"page/logs/logs",
		"tag":"学习 工作",
		"first_class": "教育",
		"second_class": "学历教育",
		"third_class": "高等",
		"first_id":3,
		"second_id":4,
		"third_id":5,
		"title": "日志"
	}
	],
	"feedback_info": "blablabla",
    "feedback_stuff": "xx|yy|zz",
    "preview_info" : {
        "video_id_list": ["xxxx"],
        "pic_id_list": ["xxxx", "yyyy", "zzzz" ]
    },
    "version_desc":"blablabla",
    "ugc_declare": {
        "scene": [
            1,
            2
        ],
        "method": [
            1
        ],
        "has_audit_team": 1,
        "audit_desc": "blablabla"
    }
}
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok",
  "auditid": 1234567
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| 0 | ok | ok |
| 61040 | "ext.json配置的隐私接口xxx无权限，请申请权限后再提交审核。或者代码中含有ext.json未配置隐私接口xxx(暂无权限)，请配置并申请权限或者承诺不使用这些接口（设置参数privacy_api_not_use为true）后再提交审核。 |  |
| 85006 | 标签格式错误 | 标签格式错误 |
| 85007 | 页面路径错误 | 页面路径错误 |
| 85008 | category is in invalid format | 当前小程序没有已经审核通过的类目，请添加类目成功后重试 |
| 85009 | already submit a version under auditing | 已经有正在审核的版本 |
| 85010 | item_list 有项目为空 | item_list 有项目为空 |
| 85011 | 标题填写错误 | 标题填写错误 |
| 85023 | item size is not in valid range | 审核列表填写的项目数不在 1-5 以内 |
| 85051 | data too large | version_desc或者preview_info超限 |
| 85077 | 小程序类目信息失效（类目中含有官方下架的类目，请重新选择类目） | 小程序类目信息失效（类目中含有官方下架的类目，请重新选择类目） |
| 85085 | submit audit reach limit pleasetry later | 小程序提审数量已达本月上限，请点击查看《自助临时申请额度》 |
| 85086 | must commit before submit audit | 提交代码审核之前需提前上传代码 |
| 85087 | 小程序已使用 api navigateToMiniProgram，请声明跳转 appid 列表后再次提交 | 小程序已使用 api navigateToMiniProgram，请声明跳转 appid 列表后再次提交 |
| 85092 | invalid preview_info format | preview_info格式错误 |
| 85093 | preview_info 视频或者图片个数超限 | preview_info 视频或者图片个数超限 |
| 85094 | need add ugc declare | 需提供审核机制说明信息 |
| 86000 | should be called only from third party | 不是由第三方代小程序进行调用 |
| 86001 | component experience version not exists | 不存在第三方的已经提交的代码 |
| 86002 | miniprogram have not completed init procedure | 小程序还未设置昵称、头像、简介。请先设置完后再重新提交 |
| 86007 | 小程序禁止提交 | 小程序禁止提交 |
| 86009 | 服务商新增小程序代码提审能力被限制 | 服务商新增小程序代码提审能力被限制 |
| 86010 | 服务商迭代小程序代码提审能力被限制 | 服务商迭代小程序代码提审能力被限制 |
| 87006 | this is game miniprogram submitaudit is forbidden | 小游戏不能提交 |
| 9400001 | 该开发小程序已开通小程序直播权限，不支持发布版本。如需发版，请解绑开发小程序后再操作。 | 该开发小程序已开通小程序直播权限，不支持发布版本。如需发版，请解绑开发小程序后再操作。 |
| 9402202 | concurrent limit | 请勿频繁提交，待上一次操作完成后再提交 |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
