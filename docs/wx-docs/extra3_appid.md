====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/developers/dev/appid.html
====================================================================================================
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

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
