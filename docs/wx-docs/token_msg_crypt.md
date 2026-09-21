====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/Message_encryption_and_decryption.html
====================================================================================================
## # 概述
 第三方平台开发者在代替授权公众号接收和处理消息时，出于安全考虑，必须对消息内容进行必须的加解密。
 本文讲述如何使用示例代码接入加解密，参考本文并使用示例代码，加解密的接入将非常简单。若想进一步的了解细节，请查看《公众号第三方平台的加密解密技术方案》
 首先请注意，开发者在接收消息和事件时，都需要进行消息加解密（某些事件可能需要回复，回复时也需要先进行加密）。但是，通过 API 主动调用接口（包括调用客服消息接口发消息）时，不需要进行加密。
 开发者可以在微信公众平台接口调试工具，在接口类型中选择“消息接口调试”，并选择安全模式的加密调试，进行消息加解密的在线调试。

### # 消息类型
 第三方平台可能会接收到两种类型的消息：
 用户发送给公众号/小程序的消息（由第三方平台代收）。此时，消息 XML 体中，ToUserName（即接收者）为公众号/小程序原始 ID（可通过获取授权方的账号基本信息接口来获得）。

 微信服务器发送给第三方平台自身的通知或事件推送（如取消授权通知，component_verify_ticket 推送等）。此时，消息 XML 体中没有 ToUserName 字段，而是 AppId 字段，即第三方平台的 AppId。这种系统事件推送通知，服务开发者收到后也需进行解密，接收到后只需直接返回字符串 success。

 具体消息加解密的做法是，当关注者与已授权公众号进行交互时，公众号第三方平台将接收到相应的消息推送、事件推送。为了加强安全性，将对此过程进行 2 个措施：
 1、在接收已授权公众号消息和事件的 URL 中，增加 2 个参数（此前已有 2 个参数，为时间戳 timestamp，随机数 nonce），分别是 encrypt_type（加密类型，为 aes）和 msg_signature（消息体签名，用于验证消息体的正确性）
 2、postdata 中的 XML 体，将使用第三方平台申请时的接收消息的加密 symmetric_key（也称为 EncodingAESKey）来进行加密。

### # 示例代码
 微信公众平台提供了 c++, php, java, python, c# 5 种语言的示例代码（点击下载，请运行示例代码前先阅读对应的 readme 文件），每种语言的类名和接口名均一致，下面以 C++ 为例说明。
 1.函数说明
 1.1. 构造函数

```
// @param sToken: 第三方平台申请时填写的接收消息的校验token
// @param sEncodingAESKey: 第三方平台申请时填写的接收消息的加密symmetric_key
// @param sAppid: 公众号第三方平台的appid

WXBizMsgCrypt(const std::string &sToken,
const std::string &sEncodingAESKey,
const std::string &sAppid)
```

1.2. 解密函数

```
// 检验消息的真实性，并且获取解密后的明文
// @param sMsgSignature: 签名串，对应URL参数的msg_signature
// @param sTimeStamp: 时间戳，对应URL参数的timestamp
// @param sNonce: 随机串，对应URL参数的nonce
// @param sPostData: 密文，对应POST请求的数据
// @param sMsg: 解密后的明文，当return返回0时有效
// @return: 成功0，失败返回对应的错误码
int DecryptMsg(const std::string &sMsgSignature,
const std::string &sTimeStamp,
const std::string &sNonce,
const std::string &sPostData,
std::string &sMsg);
```

1.3. 加密函数

```
//将公众号回复用户的消息加密打包
// @param sReplyMsg:公众号待回复用户的消息，xml格式的字符串
// @param sTimeStamp: 时间戳，可以自己生成，也可以用URL参数的timestamp
// @param sNonce: 随机串，可以自己生成，也可以用URL参数的nonce
// @param sEncryptMsg: 加密后的可以直接回复用户的密文，包括msg_signature, timestamp, nonce,encrypt的xml格式的字符串,当return返回0时有效
// return：成功0，失败返回对应的错误码
int EncryptMsg(const std::string &sReplyMsg,
const std::string &sTimeStamp,
const std::string &sNonce,
std::string &sEncryptMsg);
```

2. 使用方法
 2.1 实例化对象
 使用构造函数，实例化一个对象，传入公众号第三方平台的 token（申请公众号第三方平台时填写的接收消息的校验 token）, 公众号第三方平台的 appid, 公众号第三方平台的 EncodingAESKey（申请公众号第三方平台时填写的接收消息的加密 symmetric_key）
 2.2 解密
 安全模式下，第三方平台方收到以下带密文消息体：
 XML 格式如下：

```
encrypt_msg =
<xml>
<ToUserName></ToUserName>
<Encrypt></Encrypt>
</xml>
```

对于视频号小店的回包，则是以 json 格式返回：

```
{
  "Encrypt": "",
  "ToUserName": "",
}
```

调用示例代码中的 DecryptMsg 函数（需传入 msg_signature、timestamp、nonce 和 postdata，前 3 个参数可从接收已授权公众号消息和事件的 URL 中获得，postdata 即为 POST 过来的数据包内容），若调用成功，sMsg 则为输出结果，其内容为如下的明文的 xml 消息体:

```
<xml>
<ToUserName></ToUserName>
<FromUserName></FromUserName>
<CreateTime>1411035097</CreateTime>
<MsgType></MsgType>
<Content></Content>
<MsgId>6060349595123187712</MsgId>
</xml>
```

对于视频号小店的回包，则是以 json 格式返回：

```
{
"ToUserName": "",
"FromUserName": "",
"CreateTime": 1411035097,
"MsgType": "",
"Content": "",
"MsgId": 6060349595123187712,
}
```

2.3 公众账号处理消息，生成需要回复给微信公众平台的 xml 消息体,假设回复以下内容：

```
res_msg =
<xml>
<ToUserName></ToUserName>
<FromUserName></FromUserName>
<CreateTime>1411034505</CreateTime>
<MsgType></MsgType>
<Content></Content>
<FuncFlag>0</FuncFlag>
</xml>
```

2.4 回包加密
 调用 EncryptMsg 接口，传入需要回复给微信公众平台的 res_msg, timestamp, nonce， 若加密成功，则 sEncryptMsg 为密文消息体，内容如下：

```
<xml>
<Encrypt>
<![CDATA[LDFAmKFr7U/RMmwRbsR676wjym90byw7+hhh226e8bu6KVYy00HheIsVER4eMgz/VBtofSaeXXQBz6fVdkN2CzBUaTtjJeTCXEIDfTBNxpw/QRLGLq
qMZHA3I+JiBxrrSzd2yXuXst7TdkVgY4lZEHQcWk85x1niT79XLaWQog+OnBV31eZbXGPPv8dZciKqGo0meTYi+fkMEJdyS8OE7NjO79vpIyIw7hMBtEXPBK/tJGN5m5SoAS
6I4rRZ8Zl8umKxXqgr7N8ZOs6DB9tokpvSl9wT9T3E62rufaKP5EL1imJUd1pngxy09EP24O8Th4bCrdUcZpJio2l11vE6bWK2s5WrLuO0cKY2GP2unQ4fDxh0L4ePmNOVFJ
wp9Hyvd0BAsleXA4jWeOMw5nH3Vn49/Q/ZAQ2HN3dB0bMA+6KJYLvIzTz/Iz6vEjk8ZkK+AbhW5eldnyRDXP/OWfZH2P3WQZUwc/G/LGmS3ekqMwQThhS2Eg5t4yHv0mAIei
07Lknip8nnwgEeF4R9hOGutE9ETsGG4CP1LHTQ4fgYchOMfB3wANOjIt9xendbhHbu51Z4OKnA0F+MlgZomiqweT1v/+LUxcsFAZ1J+Vtt0FQXElDKg+YyQnRCiLl3I+GJ/c
xSj86XwClZC3NNhAkVU11SvxcXEYh9smckV/qRP2Acsvdls0UqZVWnPtzgx8hc8QBZaeH+JeiaPQD88frNvA==]]> </Encrypt>
<MsgSignature></MsgSignature>
<TimeStamp>1411034505</TimeStamp>
<Nonce></Nonce>
</xml>
```

对于视频号小店的回包，则是以 json 格式返回

```
{
  "Encrypt": "![CDATA[LDFAmKFr7U/RMmwRbsR676wjym90byw7+hhh226e8bu6KVYy00HheIsVER4eMgz/VBtofSaeXXQBz6fVdkN2CzBUaTtjJeTCXEIDfTBNxpw/QRLGLq
qMZHA3I+JiBxrrSzd2yXuXst7TdkVgY4lZEHQcWk85x1niT79XLaWQog+OnBV31eZbXGPPv8dZciKqGo0meTYi+fkMEJdyS8OE7NjO79vpIyIw7hMBtEXPBK/tJGN5m5SoAS
6I4rRZ8Zl8umKxXqgr7N8ZOs6DB9tokpvSl9wT9T3E62rufaKP5EL1imJUd1pngxy09EP24O8Th4bCrdUcZpJio2l11vE6bWK2s5WrLuO0cKY2GP2unQ4fDxh0L4ePmNOVFJ
wp9Hyvd0BAsleXA4jWeOMw5nH3Vn49/Q/ZAQ2HN3dB0bMA+6KJYLvIzTz/Iz6vEjk8ZkK+AbhW5eldnyRDXP/OWfZH2P3WQZUwc/G/LGmS3ekqMwQThhS2Eg5t4yHv0mAIei
07Lknip8nnwgEeF4R9hOGutE9ETsGG4CP1LHTQ4fgYchOMfB3wANOjIt9xendbhHbu51Z4OKnA0F+MlgZomiqweT1v/+LUxcsFAZ1J+Vtt0FQXElDKg+YyQnRCiLl3I+GJ/c
xSj86XwClZC3NNhAkVU11SvxcXEYh9smckV/qRP2Acsvdls0UqZVWnPtzgx8hc8QBZaeH+JeiaPQD88frNvA==]]",
  "MsgSignature": "",
  "TimeStamp": 1411034505,
  "Nonce":""
}
```

3. 注意事项
 1.EncodingAESKey 长度固定为 43 个字符，从 a-z,A-Z,0-9 共 62 个字符中选取。
 2.出于安全考虑，开放平台网站提供了修改 EncodingAESKey 的功能（在 EncodingAESKey 可能泄漏时进行修改），所以建议公众账号保存当前的和上一次的 EncodingAESKey，若当前 EncodingAESKey 解密失败，则尝试用上一次的 EncodingAESKey 的解密。回包时，用哪个 Key 解密成功，则用此 Key 加密对应的回包。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
