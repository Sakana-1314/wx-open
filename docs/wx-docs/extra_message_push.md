====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/message_push.html
====================================================================================================
# # 消息推送
 消息推送是开放平台推出的一种主动推送服务，基于该推送服务，开发者及时获取开放平台相关信息，无需调用API。
 总数据链路如图所示：

## # 消息推送服务器配置
 消息推送服务于小程序、公众号、小游戏、微信小店、第三方平台，这里介绍第三方平台的配置。

### # 填写相关信息
 登录微信开放平台，在「管理中心」-「第三方平台」-「开发配置」-「开发资料」中，需填写以下信息：
 URL服务器地址：开发者用来接收微信消息和事件的接口 URL，必须以 http:// 或 https:// 开头，分别支持 80 端口和 443 端口，有以下两个URL配置：
授权事件接收配置：用于接收component_verify_ticket以及授权变更通知推送。
 消息与事件接收配置：推送给第三方平台或由第三方平台代收的消息与事件，URL中可以包含“$APPID$”，推送的时候，会将代接收的小程序/公众号等的APPID填充在此。

 消息校验Token：用于签名处理，下文会介绍相关流程。
 消息加解密Key：将用作消息体加解密密钥。
 消息加解密方式：第三方平台只允许安全模式，不可选择，使用消息加解密，纯密文，安全系数高。
 数据格式：第三方平台只允许为XML，不可选择。

## # 接收消息推送
 当特定消息或事件触发时，微信服务器会将消息（或事件）的数据包以 POST 请求发送到开发者配置的 URL，下面以“debug_demo”事件为例，详细介绍整个过程：
 假设URL配置为https://www.qq.com/$APPID$/revice ，消息校验Token="AAAAA"，消息加解密Key="AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"，第三方平台Appid="wx134c8103faa5a59e"，代接收的小程序Appid="wxba5fad812f8e6fb9"。
 推送的URL链接：：https://www.qq.com/wxba5fad812f8e6fb9/revice?signature=cc0c594499c1634947d5b502f158ee518947db27&timestamp=1715943329&nonce=1590219412&openid=o9AgO5Kd5ggOC-bXrbNODIiE3bGY&encrypt_type=aes&msg_signature=6c12a4205838198b8fa631b3220723bb07f1015c
 推送的包体：

```
<xml>
    <ToUserName><![CDATA[gh_97417a04a28d]]></ToUserName>
    <Encrypt><![CDATA[D7yzvUNAL930rd28wf21s4hvXhz0L6Uit/p2Di6C5DHyYGpEgdBRnKjBec34JwoQXicwaZC7fOVihW80F4VtsdvE//1vr7oAbqjDv8KenVp+ajKYpJnyQ4zMRhIC+a31fCVOMC03FfzV/QuC94kBP55a+Za3sJgvAn+ZbsNqZI5DkyuzkhQN8OBqCzFhizGmy0xpM0MEA4agpvE+RuNO1rhHTtuJB5yltw1FiYzecSXJ+y/D2r81VkRn2eYjh2ltsoyfbDR7Is6ookXIFxTfyeNyeHxMeT4KN5WpCDmSbTcUYrbBUlkGLJ9n/rU8YOywma6G7aTb7KZKOqCxgfoUlYEZPk4FUL/TK7ShriFDMVCLyjQJ15Ob++agDWtcxhfUfe6HIoIpRW8mKNBQiY/Jd1svvuskA1wLef1RTtzKfMpagSCX/laZINmdnX4zrF6kIaR9P4xQrWXbsTFHTYe0Mg==]]></Encrypt>
</xml>
```

校验msg_signature签名是否正确，以判断请求是否来自微信服务器。注意：不要使用signature验证！
将token、timestamp（URL参数中的）、nonce（URL参数中的）、Encrypt（包体内的字段）四个参数进行字典序排序，排序后结果为:
["1590219412",
"1715943329",
"AAAAA",
"D7yzvUNAL930rd28wf21s4hvXhz0L6Uit/p2Di6C5DHyYGpEgdBRnKjBec34JwoQXicwaZC7fOVihW80F4VtsdvE//1vr7oAbqjDv8KenVp+ajKYpJnyQ4zMRhIC+a31fCVOMC03FfzV/QuC94kBP55a+Za3sJgvAn+ZbsNqZI5DkyuzkhQN8OBqCzFhizGmy0xpM0MEA4agpvE+RuNO1rhHTtuJB5yltw1FiYzecSXJ+y/D2r81VkRn2eYjh2ltsoyfbDR7Is6ookXIFxTfyeNyeHxMeT4KN5WpCDmSbTcUYrbBUlkGLJ9n/rU8YOywma6G7aTb7KZKOqCxgfoUlYEZPk4FUL/TK7ShriFDMVCLyjQJ15Ob++agDWtcxhfUfe6HIoIpRW8mKNBQiY/Jd1svvuskA1wLef1RTtzKfMpagSCX/laZINmdnX4zrF6kIaR9P4xQrWXbsTFHTYe0Mg=="]。
 将四个参数字符串拼接成一个字符串，然后进行sha1计算签名：6c12a4205838198b8fa631b3220723bb07f1015c
 与URL参数中的msg_signature参数进行对比，相等说明请求来自微信服务器，合法。

 解密消息体"Encrypt"密文。
AESKey = Base64_Decode( 消息加解密Key + "=" )，“消息加解密Key”尾部填充一个字符的 "=", 用 Base64_Decode 生成 32 个字节的 AESKey；
 将Encrypt密文进行Base64解码，得到TmpMsg， 字节长度为352
 将TmpMsg使用AESKey进行AES解密，得到FullStr，字节长度为330。AES 采用 CBC 模式，秘钥长度为 32 个字节（256 位），数据采用 PKCS#7 填充； PKCS#7：K 为秘钥字节数（采用 32），Buf 为待加密的内容，N 为其字节数。Buf 需要被填充为 K 的整数倍。在 Buf 的尾部填充(K - N%K)个字节，每个字节的内容 是(K - N%K)。微信团队提供了多种语言的示例代码（包括 PHP、Java、C++、Python、C#），请开发者尽量使用示例代码，仔细阅读技术文档、示例代码及其注释后，再进行编码调试。示例下载
 FullStr=random(16B) + msg_len(4B) + msg + appid，其中：
random(16B)为 16 字节的随机字符串；
 msg_len 为 msg 长度，占 4 个字节(网络字节序)；
 msg为解密后的明文；
 appid为第三方平台Appid，开发者需验证此Appid是否与自身第三方平台相符。

 在此示例中：
random(16B)="1205899eaf019bbd"
 msg_len=292（注意：需按网络字节序，占4个字节）
 msg=

```
<xml><ToUserName><![CDATA[gh_97417a04a28d]]></ToUserName>
<FromUserName><![CDATA[o9AgO5Kd5ggOC-bXrbNODIiE3bGY]]></FromUserName>
<CreateTime>1715943329</CreateTime>
<MsgType><![CDATA[event]]></MsgType>
<Event><![CDATA[debug_demo]]></Event>
<debug_str><![CDATA[hello world]]></debug_str>
</xml>
```

appid="wx134c8103faa5a59e"

 回包给微信服务器，首先需确定回包包体的明文内容，具体取决于特定接口文档要求，如无特定要求，回复空串或者success（无需加密）即可，其他回包内容需加密处理。这里假设回包包体的明文内容为

```
<xml><demo_resp>[CDATA[good luck]]</demo_resp></xml>
```

下面介绍如何对回包进行加密：
 回包格式如下,其中：
Encrypt：加密后的内容；
 MsgSignature：签名，微信服务器会验证签名；
 TimeStamp：时间戳；
 Nonce：随机数

```
<xml>
    <Encrypt><![CDATA[${msg_encrypt}$]]></Encrypt>
    <MsgSignature><![CDATA[${msg_signature}$]]></MsgSignature>
    <TimeStamp>${timestamp}$</TimeStamp>
    <Nonce><![CDATA[${nonce}$]]></Nonce>
</xml>
```


 Encrypt的生成方法：
AESKey = Base64_Decode( 消息加解密Key + "=" )，消息加解密Key 尾部填充一个字符的 "=", 用 Base64_Decode 生成 32 个字节的 AESKey；
 构造FullStr=random(16B) + msg_len(4B) + msg + appid，其中
random(16B)为 16 字节的随机字符串；
 msg_len 为 msg 长度，占 4 个字节(网络字节序)；
 msg为明文；
 appid为小程序Appid。

 在此示例中：
random(16B)="999951349e8ee746"
 msg_len=52（注意：需按网络字节序，占4个字节）
 msg=

```
<xml><demo_resp>[CDATA[good luck]]</demo_resp></xml>
```

appid="wx134c8103faa5a59e"
 FullStr的字节大小为90

 将FullStr用AESKey进行加密，得到TmpMsg，字节大小为96。AES 采用 CBC 模式，秘钥长度为 32 个字节（256 位），数据采用 PKCS#7 填充； PKCS#7：K 为秘钥字节数（采用 32），Buf 为待加密的内容，N 为其字节数。Buf 需要被填充为 K 的整数倍。在 Buf 的尾部填充(K - N%K)个字节，每个字节的内容 是(K - N%K)。微信团队提供了多种语言的示例代码（包括 PHP、Java、C++、Python、C#），请开发者尽量使用示例代码，仔细阅读技术文档、示例代码及其注释后，再进行编码调试。示例下载
 对TmpMsg进行Base64编码，得到Encrypt="hE8R6mGXHkJJjU72KxzKUd1GEkKJaEZq7vRL8XgK3o+00k8JGq6+pZJUIlTSyhsX+bxIBQ72g3GyvDdIZcr6+3HAZbSvPT9t/o11MI7d6WELwqrGd7jMnV0zv3Zc9Nq7"。

 TimeStamp由开发者生成，使用当前时间戳即可，示例使用1713424427。
 Nonce回填URL参数中的nonce参数即可，示例使用415670741。
 MsgSignature的生成方法：
将token、TimeStamp（回包中的）、Nonce（回包中的）、Encrypt（回包中的）四个参数进行字典序排序，排序后结果为:
["1713424427",
"415670741",
"AAAAA",
"hE8R6mGXHkJJjU72KxzKUd1GEkKJaEZq7vRL8XgK3o+00k8JGq6+pZJUIlTSyhsX+bxIBQ72g3GyvDdIZcr6+3HAZbSvPT9t/o11MI7d6WELwqrGd7jMnV0zv3Zc9Nq7"]
 将四个参数字符串拼接成一个字符串，并进行sha1计算签名：03e0812039325c2712ef5f0f980fd14c70d6e307

 最终回包为：

```
<xml>
<Encrypt><![CDATA[hE8R6mGXHkJJjU72KxzKUd1GEkKJaEZq7vRL8XgK3o+00k8JGq6+pZJUIlTSyhsX+bxIBQ72g3GyvDdIZcr6+3HAZbSvPT9t/o11MI7d6WELwqrGd7jMnV0zv3Zc9Nq7]]></Encrypt>
<MsgSignature><![CDATA[03e0812039325c2712ef5f0f980fd14c70d6e307]]></MsgSignature>
<TimeStamp>1713424427</TimeStamp>
<Nonce><![CDATA[415670741]]></Nonce>
</xml>
```

为了便于开发者调试，我们提供了相关的调试工具（请求构造、调试工具）供开发者使用。
 “请求构造”允许开发者填写相关参数后，生成debug_demo事件发包或回包的相关调试信息，供开发者使用。
 “调试工具”允许开发者填写AccessToken（包括component_access_token以及authorizer_access_token）、Body后，微信服务器会拉取你在第三方平台后台配置的消息推送配置，实际推送一条debug_demo事件供开发者调试。注意：如使用authorizer_access_token代小程序（公众号、视频号小店等）接收消息，需根据类型，将以下权限集授权给第三方平台。
| 类型 | 权限集ID |
|---|---|
| 小程序 | 171 |
| 公众号 | 172 |
| 小游戏 | 173 |
| 视频号 | 174 |


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
