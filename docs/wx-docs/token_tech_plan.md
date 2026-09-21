====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/Before_Develop/Technical_Plan.html
====================================================================================================
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

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
