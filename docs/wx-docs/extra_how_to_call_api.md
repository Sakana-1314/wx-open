====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/getting_started/how_to_call_api.html
====================================================================================================
# # 如何代商家调用接口
 在完成第三方平台账号创建、第三方平台后端服务搭建、构建授权链接并引导商家完成授权后，服务商即可获得权限代商家调用公众号、服务号、小程序、微信小店、带货助手、视频号助手的接口。

## # 调用逻辑说明
 1）简单可以理解为公众号、服务号、小程序、微信小店、带货助手、视频号助手的 API，服务商都可以调用，只是服务商调用的时候要使用 authorizer_access_token，而不是 access_token。
 2）服务商能否代商家成功调用某个公众号、服务号、小程序、微信小店、带货助手、视频号助手的 API，取决于该公众号、服务号、小程序、微信小店、带货助手、视频号助手管理员是否将对应的权限集授权给当前第三方平台账号。
 3）调用 API 出现无权限的问题，常见的错误码为 48001 和 61007，详情可查看 排错指南。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
