====================================================================================================
URL: https://developers.weixin.qq.com/doc/service/guide/dev/api/limit.html
====================================================================================================
# # 接口调用额度说明
 服务号调用接口并不是无限制的。为了防止服务号的程序错误而引发微信服务器负载异常，默认情况下，每个服务号调用接口都不能超过一定限制，当超过一定限制时，调用对应接口会收到如下错误返回码：

```
{"errcode":45009,"errmsg":"api freq out of limit"}
```

开发者可以登录微信公众平台，在账号后台开发者中心接口权限模板查看账号各接口当前的日调用上限和实时调用量，对于认证账号可以对实时调用量清零，说明如下：
 由于指标计算方法或统计时间差异，实时调用量数据可能会出现误差，一般在1%以内。
 每个账号每月共10次清零操作机会，清零生效一次即用掉一次机会（10次包括了平台上的清零和调用接口API的清零）。
 第三方帮助服务号调用时，实际上是在消耗服务号自身的quota。
 每个有接口调用限额的接口都可以进行清零操作。
 当账号粉丝数超过10W/100W/1000W时，部分接口的日调用上限会相应提升，以服务号MP后台开发者中心页面中标明的数字为准。

## # 调用额度
 新注册账号各接口调额度率限制如下：

| 接口 | 每日限额 |
|---|---|
| 获取access_token | 2000 |
| 自定义菜单创建 | 1000 |
| 自定义菜单查询 | 10000 |
| 自定义菜单删除 | 1000 |
| 创建分组 | 1000 |
| 获取分组 | 1000 |
| 修改分组名 | 1000 |
| 移动用户分组 | 100000 |
| 上传多媒体文件 | 100000 |
| 下载多媒体文件 | 200000 |
| 发送客服消息 | 500000 |
| 高级群发接口 | 100 |
| 上传图文消息接口 | 10 |
| 删除图文消息接口 | 10 |
| 获取带参数的二维码 | 100000 |
| 获取关注者列表 | 500 |
| 获取用户基本信息 | 5000000 |
| 获取网页授权access_token | 无 |
| 刷新网页授权access_token | 无 |
| 网页授权获取用户信息 | 无 |
| 设置用户备注名 | 10000 |
| 草稿箱 - 新建草稿 | 1000 |
| 草稿箱 - 获取草稿 | 500 |
| 草稿箱 - 删除草稿 | 1000 |
| 草稿箱 - 修改草稿 | 1000 |
| 草稿箱 - 获取草稿总数 | 1000 |
| 草稿箱 - 获取草稿列表 | 1000 |
| 发布能力 - 发布接口 | 100 |
| 发布能力 - 发布状态轮询接口 | 100 |
| 发布能力 - 删除发布 | 10 |
| 发布能力 - 通过 article_id 获取已发布文章 | 100 |
| 发布能力 - 获取成功发布列表 | 100 |

请注意，在测试号申请页中申请的测试号，接口调用频率限制如下：

| 接口 | 每日限额 |
|---|---|
| 获取access_token | 200 |
| 自定义菜单创建 | 100 |
| 自定义菜单查询 | 1000 |
| 自定义菜单删除 | 100 |
| 创建分组 | 100 |
| 获取分组 | 100 |
| 修改分组名 | 100 |
| 移动用户分组 | 1000 |
| 素材管理-临时素材上传 | 500 |
| 素材管理-临时素材下载 | 1000 |
| 发送客服消息 | 50000 |
| 获取带参数的二维码 | 10000 |
| 获取关注者列表 | 100 |
| 获取用户基本信息 | 500000 |
| 获取网页授权access_token | 无 |
| 刷新网页授权access_token | 无 |
| 网页授权获取用户信息 | 无 |


## # 重置 API 调用次数
 我们提供重置API调用次数接口 、重置指定API调用次数以及 使用AppSecret重置API调用次数，可对 API 调用（包括第三方帮其调用）次数进行清零，详情查看对应接口文档。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
====================================================================================================
URL: https://developers.weixin.qq.com/doc/service/en/guide/dev/api/limit.html
====================================================================================================
# # Interface frequency limit instructions
 Service Account The invocation interface is not unlimited.In order to prevent a WeChat server load exception from being caused by a program error on a service number, by default, each service number cannot call the interface beyond a certain limit. When a given limit is exceeded, the call to the corresponding interface receives the following error return code:

```
{"errcode":45009,"errmsg":"api freq out of limit"}
```

Developers can log on to the WeChat public platform and view the current daily maximum and real-time call amount of each interface of the account in the Developer Center Interface Permissions Template in the account background. For authenticated accounts, real-time calls can be zeroed, as follows:
 Due to differences in the methodology or statistical time of calculation of indicators, real-time call data may have an error, generally within 1%.
 Each account has a total of 10 zeroing opportunities per month, and once the zeroing takes effect, it is used up once (10 times include the zeroing on the platform and the zeroing of the calling interface API).
 When the third-party help Service Account is called, it is actually consuming the quota of the service number itself.
 Each interface with an interface call limit can be zeroed.
 When the number of fans of the account exceeds 10W / 100W / 1000W, the daily call limit of some interfaces will be increased accordingly, according to the number indicated in the Service Account MP background developer center page.

## # Call frequency
 The frequency limit for interface calls for newly registered accounts is as follows:

| interface | Daily limit |
|---|---|
| Get access_token | 2000 |
| Custom Menu Creating | 1000 |
| Custom Menu Queries | 10000 |
| Custom Menu Delete | 1000 |
| Create Groups | 1000 |
| Get Groups | 1000 |
| Modify group name | 1000 |
| Mobile user groups | 100000 |
| Upload multimedia files | 100000 |
| Download multimedia files | 200000 |
| Send customer service messages | 500000 |
| Advanced Group Communication Interface | 100 |
| Upload a Graphic Message Interface | 10 |
| Remove the Graphic Message Interface | 10 |
| Get a QR code with parameters | 100000 |
| Get a list of followers | 500 |
| Get basic user information | 5000000 |
| Get Web Page Authorization access_token | nothing |
| Refresh Page Authorization access_token | nothing |
| Web authorization to obtain user information | nothing |
| Set user backup name | 10000 |
| Draft Box - New drafts | 1000 |
| Draft Box - Get a draft | 500 |
| Draft Box - Delete drafts | 1000 |
| The draft box - Modify the draft | 1000 |
| Draft Box - Get the total number of drafts | 1000 |
| Draft Box - Get a list of drafts | 1000 |
| Publishing ability - Publishing interface | 100 |
| Release capability - Release status polling interface | 100 |
| Publishing ability - Delete publishing | 10 |
| Ability to publish - get published articles by article_id | 100 |
| Release Capability - Get a list of successful releases | 100 |

Note that for test numbers applied for in the test number application page, the frequency limit for interface calls is as follows:

| interface | Daily limit |
|---|---|
| Get access_token | 200 |
| Custom Menu Creating | 100 |
| Custom Menu Queries | 1000 |
| Custom Menu Delete | 100 |
| Create Groups | 100 |
| Get Groups | 100 |
| Modify group name | 100 |
| Mobile user groups | 1000 |
| Material Management - Temporary Material Upload | 500 |
| Material Management - Temporary Material Download | 1000 |
| Send customer service messages | 50000 |
| Get a QR code with parameters | 10000 |
| Get a list of followers | 100 |
| Get basic user information | 500000 |
| Get Web Page Authorization access_token | nothing |
| Refresh Page Authorization access_token | nothing |
| Web authorization to obtain user information | nothing |


## # Reset the number of API calls
 We provide Reset API Calls interface and Reset API calls with AppSecret can nullify all API calls of Service Account (including those called by third parties) for a total of 10 nullification opportunities per account per natural month, and once a nullification is effective, once an opportunity is used;
 The interface supports Service Account itself calls and Third Party Platform generation calls. The detailed interface call explaination can be found in the interface documentation.

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
