
#################### token_msg_crypt.md
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


#################### token_tech_plan.md
## # 概述
 当关注者与已授权公众号/小程序进行交互时，第三方平台将接收到相应的消息推送、事件推送。由于第三方平台一般帮助众多公众号/小程序进行业务运营，所以为了加强安全性，微信服务器将对此过程进行 2 个措施：
 在接收已授权公众号消息和事件的 URL 中，增加 2 个参数（此前已有 2 个参数，为时间戳 timestamp，随机数 nonce），分别是 encrypt_type（加密类型，为 aes）和 msg_signature（消息体签名，用于验证消息体的正确性）
 postdata 中的 XML 体，将使用第三方平台申请时的接收消息的加密 symmetric_key（也称为 EncodingAESKey）来进行加密。

### # 加密解密技术方案
 开放平台的消息加密解密技术方案基于 AES 加解密算法来实现，具体如下：
 EncodingAESKey： 即消息加解密 Key，长度固定为 43 个字符，从 a-z,A-Z,0-9 共 62 个字符中选取。由开发者在创建公众号插件时填写，后也可申请修改。

 AESKey： AESKey=Base64_Decode(EncodingAESKey + "=")，EncodingAESKey 尾部填充一个字符的 "=", 用 Base64_Decode 生成 32 个字节的 AESKey；

 AES 采用 CBC 模式，秘钥长度为 32 个字节（256 位），数据采用 PKCS#7 填充； PKCS#7：K 为秘钥字节数（采用 32），Buf 为待加密的内容，N 为其字节数。Buf 需要被填充为 K 的整数倍。在 Buf 的尾部填充(K - N%K)个字节，每个字节的内容 是(K - N%K)。


| 尾部填充 | 说明 | 示例 |
|---|---|---|
| 01 | if ( N%K==(K-1)) |  |
| 0202 | if ( N%K==(K-2)) |  |
| 030303 | if ( N%K==(K-3)) |  |
| ... | ... |  |
| KK....KK (K 个字节) | if ( N%K==0) |  |

具体详见：http://tools.ietf.org/html/rfc2315
 BASE64 采用 MIME 格式，字符包括大小写字母各 26 个，加上 10 个数字，和加号 "+"，斜杠 "/"，一共 64 个字符，等号 "=" 用作后缀填充；

 出于安全考虑，开放平台网站提供了修改 EncodingAESKey 的功能（在 EncodingAESKey 可能泄漏时进行修改，对应上第三方平台申请时填写的接收消息的加密 symmetric_key），所以建议开放平台账号保存当前的和上一次的 EncodingAESKey，若当前 EncodingAESKey 生成的 AESKey 解密失败，则尝试用上一次的 AESKey 的解密。回包时，用哪个 AESKey 解密成功，则用此 AESKey 加密对应的回包。

 微信团队提供了多种语言的示例代码（包括 PHP、Java、C++、Python、C#），请开发者尽量使用示例代码，仔细阅读技术文档、示例代码及其注释后，再进行编码调试。示例下载

### # 接收授权方的用户消息
 下面以普通文本消息为例，详细说明公众平台对消息体加解密的方法和流程，其它普通消息和事件消息的加解密可以此类推。

#### # 消息体加密
 现有消息为明文，格式如下：

```
<xml>
  <ToUserName><![CDATA[示例内容]]></ToUserName>
  <FromUserName><![CDATA[示例内容]]></FromUserName>
  <CreateTime>1348831860</CreateTime>
  <MsgType><![CDATA[示例内容]]></MsgType>
  <Content><![CDATA[示例内容]]></Content>
  <MsgId>1234567890123456</MsgId>
</xml>
```

对于视频号小店的回包，则是以 json 格式返回：

```
{
  "ToUserName": "",
  "FromUserName": "",
  "CreateTime": 1348831860,
  "MsgType": "",
  "Content": "",
  "MsgId": 1234567890123456
}
```

加密后，消息格式如下:

```
<xml>
  <ToUserName><![CDATA[toUser]]></ToUserName>
  <Encrypt><![CDATA[msg_encrypt]]></Encrypt>
</xml>
```

对于视频号小店的回包，则是以 json 格式返回：

```
{
  "ToUserName": "",
  "Encrypt": ""
}
```

其中，msg_encrypt是由平台对消息做了如下加密处理后的结果：
 AESKey = Base64_Decode(EncodingAESKey + "=");
 FullStr = random(16B) + msg_len(4B) + msg + appid;
 msg_encrypt = Base64_Encode( AES_Encrypt( FullStr, AESKey ) );

#### # 消息体签名
 为了验证消息体的合法性，开放平台新增消息体签名，开发者可用以验证消息体的真实性，并对验证通过的消息体进行解密。具体做法如下：在微信服务器向公众号插件推送消息时，将会在其消息接收 URL（创建时填写）上增加参数：msg_signature
msg_signature=sha1(sort(Token、timestamp、nonce, msg_encrypt))

| 参数 | 描述 |
|---|---|
| Token | 微信开放平台上，服务方设置的接收消息的校验 token |
| timestamp | URL 上原有参数,时间戳 |
| nonce | URL 上原有参数,随机数 |
| msg_encrypt | 前文描述密文消息体 |


#### # 消息体验证和解密
 开发者先验证消息体签名的正确性，验证通过后，再对消息体进行解密。

#### # 验证方式：

```
1. 开发者计算签名，dev_msg_signature=sha1(sort(Token、timestamp、nonce, msg_encrypt))

2. 比较dev_msg_signature和URL上带的msg_signature是否相等，相等则表示验证通过。
```


#### # 解密方式如下：

```
1. TmpMsg = Base64_Decode(msg_encrypt) 

2. FullStr = AES_Decrypt(TmpMsg, AESKey);  FullStr 如前所述由4部分组成（random, msg_len, msg, appid）

3. 验证尾部的appid 是否正确（可选）

4. 去掉FullStr头部16字节的random、4字节的msg_len、和尾部的appid，即得到明文内容
```


#### # 四、例子：服务方代替授权方向用户回复消息

##### # 回复消息体的签名与加密
 现有消息格式：

```
<xml>
  <ToUserName></ToUserName>
  <FromUserName></FromUserName>
  <CreateTime>12345678</CreateTime>
  <MsgType></MsgType>
  <Content></Content>
</xml>
```

对于视频号小店的回包，则是以 json 格式返回：

```
{
  "ToUserName": "",
  "FromUserName": "",
  "CreateTime": 12345678,
  "MsgType": "",
  "Content": ""
}
```

加密后消息格式：

```
<xml>
  <Encrypt></Encrypt>
  <MsgSignature></MsgSignature>
  <TimeStamp></TimeStamp>
  <Nonce></Nonce>
</xml>
```

对于视频号小店的回包，则是以 json 格式返回：

```
{
  "Encrypt": "",
  "MsgSignature": "",
  "TimeStamp": 1348831860,
  "Nonce": ""
}
```

其中，msg_encrypt = Base64_Encode( AES_Encrypt( FullStr, AESKey ) );
 FullStr = random(16B) + msg_len(4B) + msg + appid;
 AESKey = Base64_Decode(EncodingAESKey + "=");
 FullStr 中，
 random(16B)为 16 字节的随机字符串；
 msg_len 为 msg 长度，占 4 个字节(网络字节序)；
 msg 为服务方回复的内容；
 appid 为服务方的appid；
 此外，msg_signature=sha1(sort(Token、timestamp、nonce, msg_encrypt))，timestamp、nonce 回填请求中的值即可。

##### # 常见错误举例
 对开发者在进行消息加解密过程中可能会遇到的常见错误问题，整理原因如下：
 xml 格式不对:如写成了 (s 小写了且 p 和>中间有空格）；
 公众平台网站提供了修改 EncodingAESKey 的功能，公众账号需要保存当前的和上一次的 EncodingAESKey，若当前的 EncodingAESKey 解密失败，则尝试用上一次的 EncodingAESKey 解密。回包时，用哪个 Key 解密成功，则用此 Key 加密对应的回包。
 java 要求 jdk 1.6 以上；
 异常 java.security.InvalidKeyException:illegal Key Size 的解决方案：在官方网站下载 JCE 无限制权限策略文件（JDK7 的下载地址）
 下载后解压，可以看到 local_policy.jar 和 US_export_policy.jar 以及 readme.txt，如果安装了 JRE，将两个 jar 文件放到%JRE_HOME%\lib\security 目录下覆盖原来的文件；如果安装了 JDK，将两个 jar 文件放到%JDK_HOME%\jre\lib\security 目录下覆盖原来文件


#################### token_call_interface.md
# # 代调用接口介绍
 第三方平台代公众号、服务号、小程序、微信小店、带货助手、视频号助手调用接口指的是，第三方平台在获得公众号、服务号、小程序、微信小店、带货助手、视频号助手管理员的授权之后，第三方平台则可以以公众号、服务号、小程序、微信小店、带货助手、视频号助手的身份调用已获得权限的接口。

## # 调用接口之前注意事项
 然而，第三方平台在帮助公众号、服务号、小程序、微信小店、带货助手、视频号助手调用接口之前，需要先确认：
 1、获得了该公众号、服务号、小程序、微信小店、带货助手、视频号助手的该接口权限的授权，否则会出现61007的报错。
 2、该公众号、服务号、小程序、微信小店、带货助手、视频号助手自身拥有该接口权限，否则会出现48001的报错

## # 调用接口之后注意事项
 1、获得权限后即可调用接口，如果接口的调用次数超出了额度（出现45009错误码），则可以调用 clear_quota 接口进行次数清零。
 2、然而，每个账号每个月有 10 次清零机会，包括在微信公众平台上的清零以及调用 API 进行清零。如果次数用完了，则会出现48006报错。
 3、如果在授权公众号以及第三方平台的开发和业务运营过程中遇到报警时，请参阅报警排查指引来解决问题：报警排查指引
 4、最后，如果使用过程中如遇到问题，可在开放平台服务商专区发帖交流


#################### auth_auth_process.md
# # 授权流程技术说明

## # 一、概述

### # （1）整体说明
 1、本文涉及的小程序或者公众号授权给第三方平台的技术实现流程仅适用于平台型第三方平台，不适用于定制化型第三方平台。
 2、当前提供了三种授权方式，分别是PC版扫码授权、H5版授权及小程序插件版授权，开发者可根据自身业务情况，选择合适的授权方式。
 3、服务商获取商家授权是“服务商为商家提供服务”的基础，服务商可以按照下方文档说明构建授权链接与授权码，亦可以通过“一键部署官方提供的第三方平台云服务”的方式获得系统自动生成的授权链接与授权码。
 4、完成一次完整的授权，需要服务商与商家的配合，相关流程如下。

### # （2）自建授权链接
 步骤一、前往微信开放平台-第三方平台-详情-开发配置，完成权限集与开发资料的配置。
 步骤二、调用接口获取预授权码（pre_auth_code），接口详情请查看api_create_preauthcode
 步骤三、准备“授权回调 URI”，然后按照官方文档规则生成PC端的授权二维码或者移动端的授权链接，详情请看下方说明
 步骤四、公众号/小程序管理员扫码或者访问移动端授权链接，确认同意授权给第三方平台。（如果该第三方平台账号尚未全网发布，则需要先将要用于测试的公众号或者小程序加入第三方平台-开发资料的“授权测试公众号/小程序列表”。）
 步骤五、管理员授权确认之后，授权页会自动跳转进入回调 URI，并在 URL 参数中返回授权码和过期时间(redirect_url?auth_code=xxx&expires_in=600)。
 步骤六、调用接口生成authorizer_access_token，然后以该token调用公众号或小程序的相关 API。

### # （3）使用官方云服务生成授权链接
 步骤一、创建第三方平台账号时选择云服务，然后一键部署、一键配置，完成搭建。
 步骤二、前往“服务商微管家”-管家中心-授权链接生成器，即可复制已经生成的授权链接和授权码。其余步骤，同上。

### # （4）补充说明
 服务商能代商家调用哪些 API，取决于商家将哪些权限集授权给了服务商，也取决于公众号或小程序自身拥有哪些接口权限，使用 JS SDK 等能力。

 如果商家需要解除授权，则需要登录微信公众平台进行操作；当前不支持第三方服务商主动解除授权。

 如果小程序/公众号管理员只想取消对服务商的个别权限集的授权，则可以重新扫码进入授权页面，然后自定义权限集，重新授权即可。（如果是小程序商家，则可以前往微信公众平台 - 设置-第三方设置-管理授权，进入更新授权页面后取消部分授权即可）

 当官方开放新的权限集，服务商可前往第三方平台增加新的权限，以满足新的业务需求。修改后新授权的公众号/小程序授权时会增加新权限的申请；已授权的老用户，旧有权限不影响，但新权限集需要商家重新扫描授权升级后才可获得。

 权限集只可以授权给公众号或者普通小程序和小商店，其他类型的账号暂时不支持。可通过api_get_authorizer_info查看账号的类型。

### # （5）视频号授权说明
 服务商构建授权链接或者授权码后可发送给商家进行扫码或选择授权账号，将指定的授权账号授权与服务商；如果选择的授权账号为视频号，则商家需前往视频号控制台 - 直播管理 - 开放能力，开通开放能力后方可授权
 开通相关能力后的交互可参考如下（并且可在该控制台上解除与服务商的授权）：

## # 二、授权链接构建的方法

### # 1、授权链接参数说明

| 参数 | 必填 | 说明 |
|---|---|---|
| component_appid | 是 | 第三方平台方 appid |
| pre_auth_code | 是 | 预授权码 |
| redirect_uri | 是 | - 授权回调 URI(填写格式为https://xxx)。（插件版无该参数） - 管理员授权确认之后会自动跳转进入回调 URI，并在 URL 参数中返回授权码和过期时间(redirect_url?auth_code=xxx&expires_in=600) |
| auth_type | 是 | - 要授权的账号类型，即商家点击授权链接或者扫了授权码之后，展示在用户手机端的授权账号类型。 - 1 表示手机端仅展示公众号；2 表示仅展示小程序，3 表示公众号和小程序都展示。 - 4表示小程序推客账号； - 5表示视频号账号； - 6表示全部，即公众号、小程序、视频号都展示 - 8表示带货助手账号； - 第三方平台开发者可以使用本字段来控制授权的账号类型。 - 对于已经注销、冻结、封禁、以及未完成注册的账号不再出现于授权账号列表。 |
| biz_appid | 否 | - 指定授权唯一的小程序或公众号 。 - 如果指定了appid，则只能是该appid的管理员进行授权，其他用户扫码会出现报错。 - auth_type、biz_appid 两个字段如果设置的信息冲突，则biz_appid生效的优先级更高。 - 例如，auth_type=1，但是biz_appid是小程序的appid，则会按照auth_type=2来处理，即以biz_appid的类型为准去拉出来对应的权限集列表. |
| category_id_list | 否 | - 指定的权限集id列表，如果不指定，则默认拉取当前第三方账号已经全网发布的权限集列表。 - 如需要指定单个权限集ID，写法为“category_id_list=99” ，如果有多个权限集，则权限集id与id之间用中竖线隔开。 |


### # 2、授权列表页逻辑说明
 当商家进入了授权账号列表页展示的账号列表，除了受“授权参数”影响，还与第三方平台账号的配置有关，以及与微信号用户的角色有关。

### # 3、不同类型授权链接使用场景

| 版本 | 使用场景 |
|---|---|
| PC版 | - 访问PC版授权链接则会自动出现授权码，商家以微信扫码的方式进入授权账号列表页，选择账号以完成授权。 - 通常将该链接放置服务商PC版官网或者saas业务控制台中 |
| H5版 | - 访问H5版授权链接则直接进入授权账号列表页，选择账号以完成授权。 - 通常将该链接放置服务商H5版（例如服务号）官网或者saas业务控制台中 |
| 插件版 | - 商家可在服务商小程序里直接进入授权账号列表页，选择账号以完成授权。 - 通常将该链接放置服务商小程序版官网或者saas业务控制台中 |


### # 4、授权链接拼接方式

| 版本 | 使用场景 |
|---|---|
| PC版 | https://mp.weixin.qq.com/cgi-bin/componentloginpage?component_appid=xxxx&pre_auth_code=xxxxx&redirect_uri=xxxx&auth_type=xxx |
| H5版-新版 | https://open.weixin.qq.com/wxaopen/safe/bindcomponent?action=bindcomponent&no_scan=1&component_appid=xxxx&pre_auth_code=xxxxx&redirect_uri=xxxx&auth_type=xxx&biz_appid=xxxx#wechat_redirect |


### # 5、插件版使用方式
 需先申请“服务商组件”方可使用插件版授权页，关于小程序服务商组件到更多使用指南，请查看https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/product/Register_Mini_Programs/beta_mp_plugin.html。

#### # 代码示例

```
const MiniprogramThirdpartyPlugin = requirePlugin('miniprogram-thirdparty-plugin')  

// 初始化 
MiniprogramThirdpartyPlugin.init(wx)  

// 请求用户授权 
MiniprogramThirdpartyPlugin.openAuthorizeAccount({
   platformAppID: '', 
   preAuthCode：‘’，//获取的预授权码
   authType：3， 
   bizAppid: wxxxxxxxxx,//非必填字段，参数详情请看文章末尾
   })
```


#### # 插件版参数说明

| 参数 | 必填 |  |
|---|---|---|
| platformAppID | 是 | 第三方平台方 appid |
| preAuthCode | 是 | 预授权码，可通过pre_auth_code接口获得 |
| authType | 是 | 要授权的账号类型：1 则商户点击链接后，手机端仅展示公众号、2 表示仅展示小程序，3 表示公众号和小程序都展示。如果为未指定，则默认小程序和公众号都展示。第三方平台开发者可以使用本字段来控制授权的账号类型。 |
| bizAppid | 否 | 指定授权唯一的小程序或公众号 |


## # 三、商家的使用步骤
 可前往授权相关操作进行查看。

