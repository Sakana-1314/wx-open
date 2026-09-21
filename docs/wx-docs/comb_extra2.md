
#################### extra2_basic.md
# # 使用分包

## # 配置方法
 假设支持分包的小程序目录结构如下：

```
├── app.js
├── app.json
├── app.wxss
├── packageA
│   └── pages
│       ├── cat
│       └── dog
├── packageB
│   └── pages
│       ├── apple
│       └── banana
├── pages
│   ├── index
│   └── logs
└── utils
```

开发者通过在 app.json subPackages 字段声明项目分包结构：
 写成 subpackages 也支持。

```
{
  "pages":[
    "pages/index",
    "pages/logs"
  ],
  "subPackages": [
    {
      "root": "packageA",
      "pages": [
        "pages/cat",
        "pages/dog"
      ],
      "entry": "index.js"
    }, {
      "root": "packageB",
      "name": "pack2",
      "pages": [
        "pages/apple",
        "pages/banana"
      ]
    }
  ]
}
```

subPackages 中，每个分包的配置有以下几项：

| 字段 | 类型 | 说明 |
|---|---|---|
| root | String | 分包根目录 |
| name | String | 分包别名，分包预下载时可以使用 |
| pages | StringArray | 分包页面路径，相对于分包根目录 |
| independent | Boolean | 分包是否是独立分包 |
| entry | String | 分包入口文件 |


## # 打包原则
 声明 subPackages 后，将按 subPackages 配置路径进行打包，subPackages 配置路径外的目录将被打包到主包中
 主包也可以有自己的 pages，即最外层的 pages 字段。
 subPackages 的根目录不能是另外一个 subPackages 内的子目录
 tabBar 页面必须在主包内

## # 引用原则
 packageA 无法 require packageB JS 文件，但可以 require 主包、packageA 内的 JS 文件；使用 分包异步化 时不受此条限制
 packageA 无法 import packageB 的 template，但可以 require 主包、packageA 内的 template
 packageA 无法使用 packageB 的资源，但可以使用主包、packageA 内的资源

## # 分包入口文件
 每个分包的配置中，entry 字段可以指定该分包中的任意一个 JS 文件作为入口文件，该文件会在分包注入时首先被执行。
 指定的 JS 文件应该填写相对于分包根目录的路径，例如需要指定 /path/to/subPackage/src/index.js 作为分包 /path/to/subPackage 的入口文件时，应填写 src/index.js。
 调试这个功能需要 1.06.2406242 或以上版本的微信开发者工具，正式环境没有版本需求。
 在开发者工具中预览效果

## # 示例项目
 下载小程序示例（分包加载版）源码


#################### extra2_submit_quota.md
# # 第三方小程序提审问题
 服务商通过 API 代商家提交小程序代码审核，有一定的 quota，详细机制说明可查看第三方服务商提 quota 机制优化说明。
 如果 quota 不足继续调用提交审核接口，会出现 85085 报错。
 如果遇到 quota 不足，或遇到审核相关的问题，可在「小程序服务商助手」小程序中提交申请或找到人工客服进行咨询。小程序服务商助手的小程序码如下：


#################### extra2_Return_code_descriptions.md
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

#################### extra3_submit_audit.md
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



#################### extra3_appid.md
# # 如何查看和重置 AppSecret
 access_token 是调用微信服务端接口的凭证，生成 access_token 则需要 AppID 和 AppSecret 信息，开发者可查看下文了解如何获取 AppID 和 AppSecret 信息。并且平台还支持冻结/解除冻结、重置 AppSecret。
 注意：
 1）平台不储存和显示 AppSecret，如已忘记 AppSecret，则可通过「重置」功能重新生成开发者密钥（AppSecret），并需妥善保存。
 2）如果重置 AppSecret，旧的 AppSecret 会失效，因此需要慎重。即，在重置 AppSecret 时应当梳理清楚旧的 AppSecret 在哪些业务系统有使用，以及为了确保账号的正常使用，需尽快更新 AppSecret 信息。

## # 1、小程序和小游戏
 小程序和小游戏的 AppID 和 AppSecret 信息可在微信公众平台获取，具体操作如下：
 前往微信公众平台，选择小程序/小游戏账号登录，点击左侧菜单栏的「管理 - 开发管理」，进入「开发管理」页面，即可查看 AppID 和 AppSecret 信息。并且可在此重置或者冻结/解除冻结 AppSecret。

## # 2、公众号和服务号以及带货助手
 公众号和服务号以及带货助手的 AppID 和 AppSecret 信息可在微信开发者平台获取，具体操作如下：
 前往微信开发者平台，使用微信扫码登录，点击「我的业务 - 公众号/服务号/带货助手」
  然后，进入详情页，即可查看 AppID 和 AppSecret 信息。并且可在此重置或者冻结/解除冻结 AppSecret（带货助手的 AppSecret 暂不支持冻结）。

## # 3、移动应用和网站应用以及第三方平台
 移动应用和网站应用以及第三方平台的 AppID 和 AppSecret 信息可在微信开放平台获取，具体操作如下：
 前往微信开放平台，使用邮箱密码登录，然后进入管理中心，分别再进入移动应用和网站应用以及第三方平台详情页，即可查看 AppID 和 AppSecret 信息，并且可在此重置 AppSecret 信息。

## # 4、微信小店
 前往「微信小店 - 服务市场 - 经营工具 - 自研」即可查看 AppID 和 AppSecret 信息。并且可在此重置 AppSecret。

## # 5、视频号助手
 前往「视频号助手 - 直播 - 直播管理 - 开放能力」即可查看 AppID 和 AppSecret 信息。并且可在此重置 AppSecret。

## # 6、联盟带货机构
 前往「微信小店 · 联盟带货机构管理平台 - 设置 - 开放信息」即可查看 AppID 和 AppSecret 信息。并且可在此重置 AppSecret。

