====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/code/submit_audit.html
====================================================================================================
## # 提交审核
 在调用上传代码接口为小程序上传代码后，可以调用本接口，将上传的代码提交审核。使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

### # 请求地址

```
POST https://api.weixin.qq.com/wxa/submit_audit?access_token=ACCESS_TOKEN
```


### # 请求参数说明

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| access_token | String | 是 | 第三方平台接口调用令牌authorizer_access_token |
| item_list | Object Array | 是 | 审核项列表（选填，至多填写 5 项）；类目是必填的，且要填写已经在小程序配置好的类目 |
| preview_info | Object | 否 | 预览信息（小程序页面截图和操作录屏） |
| version_desc | String | 否 | 小程序版本说明和功能解释 |
| feedback_info | String | 否 | 反馈内容，至多 200 字 |
| feedback_stuff | String | 否 | 用 | 分割的 media_id 列表，至多 5 张图片, 可以通过新增临时素材接口上传而得到 |
| ugc_declare | Object | 否 | 用户生成内容场景（UGC）信息安全声明 |

注意：只有上个版本被驳回，才能使用 feedback_info、feedback_stuff 这两个字段，否则忽略处理。
 当小程序第一次提交审核且类目包含社交-社区/论坛、社交-笔记、社交-问答其中之一时需填写 ugc_declare

#### # 审核项说明

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| address | String | 否 | 小程序的页面，可通过获取小程序的页面列表接口获得 |
| tag | String | 否 | 小程序的标签，用空格分隔，标签至多 10 个，标签长度至多 20 |
| first_class | String | 是 | 一级类目名称 |
| second_class | String | 是 | 二级类目名称 |
| third_class | String | 否 | 三级类目名称 |
| first_id | number | 是 | 一级类目的 ID |
| second_id | number | 是 | 二级类目的 ID |
| third_id | number | 否 | 三级类目的 ID |
| title | String | 否 | 小程序页面的标题,标题长度至多 32 |

注意：
first_class/second_class/third_class、first_id/second_id/third_id 通过获取审核时可填写的类目信息接口 获得

#### # 预览信息说明

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| video_id_list | String Array | 否 | 录屏mediaid列表，可以通过提审素材上传接口获得 |
| pic_id_list | String Array | 否 | 截屏mediaid列表，可以通过提审素材上传接口获得 |


#### # 用户生成内容场景（UGC）信息安全声明

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| scene | Number Array | 否 | UGC场景 0,不涉及用户生成内容, 1.用户资料,2.图片,3.视频,4.文本,5其他, 可多选,当scene填0时无需填写下列字段 |
| other_scene_desc | String | 否 | 当scene选其他时的说明,不超时256字 |
| method | Number Array | 否 | 内容安全机制 1.使用平台建议的内容安全API,2.使用其他的内容审核产品,3.通过人工审核把关,4.未做内容审核把关 |
| has_audit_team | Number | 否 | 是否有审核团队, 0.无,1.有,默认0 |
| audit_desc | String | 否 | 说明当前对UGC内容的审核机制,不超过256字 |

POST 数据示例:

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


### # 返回参数说明

| 参数 | 类型 | 说明 |
|---|---|---|
| errcode | Number | 返回码 |
| errmsg | String | 错误信息 |
| auditid | String | 审核编号 |

返回结果示例：

```
{
  "errcode": 0,
  "errmsg": "ok",
  "auditid": 1234567
}
```


### # 返回码说明

| 返回码 | 说明 |
|---|---|
| -1 | 系统繁忙 |
| 0 | 成功 |
| 86000 | 不是由第三方代小程序进行调用 |
| 86001 | 不存在第三方的已经提交的代码 |
| 85006 | 标签格式错误 |
| 85007 | 页面路径错误 |
| 85008 | 添加的类目不属于该小程序已经配置的类目，请检查配置后重新提交 |
| 85009 | 已经有正在审核的版本 |
| 85010 | item_list 有项目为空 |
| 85011 | 标题填写错误 |
| 85023 | 审核列表填写的项目数不在 1-5 以内 |
| 85077 | 小程序类目信息失效（类目中含有官方下架的类目，请重新选择类目） |
| 86002 | 小程序还未设置昵称、头像、简介。请先设置完后再重新提交 |
| 85085 | 小程序提审数量已达本月上限，请点击查看《临时quota申请流程》 |
| 85086 | 提交代码审核之前需提前上传代码 |
| 85087 | 小程序已使用 api navigateToMiniProgram，请声明跳转 appid 列表后再次提交 |
| 87006 | 小游戏不能提交 |
| 86007 | 小程序禁止提交 |
| 85051 | version_desc或者preview_info超限 |
| 85092 | preview_info格式错误 |
| 85093 | preview_info 视频或者图片个数超限 |
| 85094 | 需提供审核机制说明信息 |
| 86009 | 服务商新增小程序代码提审能力被限制 |
| 86010 | 服务商迭代小程序代码提审能力被限制 |
| 9400001 | 该开发小程序已开通小程序直播权限，不支持发布版本。如需发版，请解绑开发小程序后再操作。 |
| 9402202 | 请勿频繁提交，待上一次操作完成后再提交 |
| 其他错误码 | 请查看全局错误码 |


### # 提审素材上传接口介绍
 调用本接口，将小程序页面截图和操作录屏上传，提审时带上相关参数，可以帮助审核人员判断。
http请求方式：POST/FORM，使用https。请使用第三方平台接口调用令牌authorizer_access_token

### # 请求地址

```
https://api.weixin.qq.com/wxa/uploadmedia?access_token=ACCESS_TOKEN
```

调用示例（使用curl命令，用FORM表单方式上传一个多媒体文件）:

```
curl -F media=@test.jpg "https://api.weixin.qq.com/wxa/uploadmedia?access_token=ACCESS_TOKEN"
curl -F media=@xxx.mp4 "https://api.weixin.qq.com/wxa/uploadmedia?access_token=ACCESS_TOKEN"
```

返回结果示例：

```
{
	"errcode":0,
	"errmsg":"ok",
	"type":"image",
	"mediaid":"xxxxxxxxxxxxxxxxx"
}
```

图片（image）: 2M，支持PNG\JPEG\JPG\GIF格式
视频（video）：10MB，支持MP4格式
完成素材上传后，使用返回的mediaid，可以在提审接口通过post preview_info完成图片和视频上传。
注意：返回的mediaid有效期是三天，过期需要重新上传

### # 返回码说明

| 返回码 | 说明 |
|---|---|
| -1 | 系统繁忙 |
| 0 | 成功 |
| 43002 | 需要用post请求 |
| 41005 | 传输素材无视频或图片内容 |
| 40005 | 上传素材文件格式不对 |
| 40006 | 上传素材文件大小超出限制 |
| 86020 | 小程序名称不合法，请把“的试用小程序”、“的试用店”等信息去掉，修改名次后重试 |
| 85085 | 小程序提审数量已达本月上限，请点击查看《自助临时申请额度》 |
| 其他错误码 | 请查看全局错误码 |


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
