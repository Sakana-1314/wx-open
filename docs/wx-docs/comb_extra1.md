
#################### extra_message_push.md
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



#################### extra_how_to_call_api.md
# # 如何代商家调用接口
 在完成第三方平台账号创建、第三方平台后端服务搭建、构建授权链接并引导商家完成授权后，服务商即可获得权限代商家调用公众号、服务号、小程序、微信小店、带货助手、视频号助手的接口。

## # 调用逻辑说明
 1）简单可以理解为公众号、服务号、小程序、微信小店、带货助手、视频号助手的 API，服务商都可以调用，只是服务商调用的时候要使用 authorizer_access_token，而不是 access_token。
 2）服务商能否代商家成功调用某个公众号、服务号、小程序、微信小店、带货助手、视频号助手的 API，取决于该公众号、服务号、小程序、微信小店、带货助手、视频号助手管理员是否将对应的权限集授权给当前第三方平台账号。
 3）调用 API 出现无权限的问题，常见的错误码为 48001 和 61007，详情可查看 排错指南。


#################### extra_template.md
# # 小程序模板库管理
 第三方代开发小程序的路径如下图所示。将代码提交到草稿箱之后，需要将其添加至模板库才可以提交代码。小程序模板库用于存储开发者开发的小程序代码。如果使用 directCommit 模式提交代码，则不需要经过模板库，详情可查看第三方小程序开发模式说明。
  限制说明
 1）小程序模板库的存储数量上限为 200 个。
 2）由于标准模板依赖交易组件，但交易组件已下架，因此建议开发者使用普通模板（标准模板已无法继续使用）。平台近期也会对标准模板进行下架处理。
 相关接口

| 接口 | 说明 |
|---|---|
| 获取代码草稿列表 | 获取当前草稿箱中的代码草稿列表 |
| 将草稿添加到代码模板库 | 将草稿箱中的草稿添加至代码模板库 |
| 获取代码模板列表 | 获取当前账号下的代码模板列表 |
| 删除指定代码模板 | 删除代码模板库中的指定模板 |



#################### extra_how_to_dev.md
# # 服务商代开发小程序

## # 提交代码审核前的前置检查项补充说明
 除了上图中提到的名称、简介、类目和头像，还需检查用户隐私保护指引是否已经配置好，相关配置指引可查看配置小程序用户隐私保护指引；公告详情可查看《关于补充小程序、插件用户隐私保护指引说明》。
 此外，如果小程序还涉及申请地理位置等相关隐私接口，还需对相关 API 进行权限申请、在代码中进行声明，相关公告详情可分别查看小程序地理位置相关接口调整、地理位置接口新增与相关流程调整。
 上传代码以及提交代码审核接口的注意事项，可查看对应的接口文档。

## # 一、代开发小程序步骤说明
 如上图所示，第三方平台帮助旗下已授权的小程序进行代码管理时，需先开发完成小程序模板，再将小程序模板部署到旗下小程序账号中，具体流程如下：
 第一步：绑定开发小程序
 1）第三方平台的开发人员需先到微信公众平台申请一个普通的小程序并完善小程序的头像、昵称、简介、服务类目等信息。
 2）进入微信开放平台，在第三方平台详情中，将该小程序添加为开发小程序。注意，绑定为开发小程序后，该小程序在开发者工具中上传的代码会直接上传到开放平台，不会上传到公众平台。
 第二步：小程序模板的开发和上传
 使用开发小程序的开发者微信号登录微信开发者工具，按照正常的小程序开发流程进行代码开发和调试。开发完成后，在开发者工具中点击上传。使用详见：第三方平台代开发小程序。
 第三步：添加到小程序模板库，获得模板 ID
 从开发者工具中上传的代码，会先存在草稿箱中，每个开发小程序只保留最新一份上传记录。开发者可将草稿箱中的代码添加到小程序模板库中，小程序模板库中的模板不会被覆盖。最多可以有 200 个代码模板，添加后可以获得模板 ID（TemplateID）。
 第四步：调用接口，为旗下授权的小程序部署代码
 具体接口详见上传代码。
 注意：小程序授权托管之后，只能使用第三方平台在微信开放平台登记的服务器域名和业务域名。因此，第三方平台在帮助旗下小程序发布代码之前，需先把服务器域名和业务域名设置到小程序中。设置接口详见设置服务器域名和设置业务域名。

## # 二、代开发模式提交小程序代码方式说明
  绑定开发小程序的操作，请查看绑定开发小程序。
 更多关于第三方平台代开发小程序介绍可查看 ext 开发文档。
 上述流程是将代码提交到草稿箱，再到模板库，再提交到小程序。如果想实现直接将代码提交到小程序，则可以通过使用 directCommit 直接提交至待审核列表。directCommit 是 ext.json 里的一个参数，详情可查看 extAppid 的开发调试。
 除了通过开发者工具提交代码，还可以通过 miniprogram-ci 提交代码，directCommit 同样适用于 CI 工具。
  使用第三方代开发模式，重点需要关注 ext.json 文件的使用，详情可查看 ext 开发文档。

## # 三、为什么选择第三方平台进行小程序代开发
 如上图所示，第三方小程序的开发步骤看起来比普通模式多了一些，那么为什么还是建议服务商选择第三方平台的方式进行代开发小程序呢？

### # 效率更高
 一个小程序模板可以批量提交给千千万万的商家小程序。
 当小程序版本需要更新时，也可以批量更新。
 开发和管理的效率提高了，也间接降低了开发和管理的成本。

### # 更可靠
 当商家小程序将开发权限授权给服务商之后，商家登录公众平台也将无法进行版本管理、域名配置等操作，一定程度上避免由于商家的误操作导致小程序业务故障或不稳定。

### # 更快速
 平台方在逐步推进更多的场景可按照小程序模板进行审核，模板审核通过之后，且商家小程序满足相关条件，则可加速通过审核。

## # 其他常见问题
 第三方小程序审核规则、加急等操作指引请查看第三方服务商提审限额机制优化说明。


#################### extra_config.md
# # 第三方平台开发配置
 关于开发资料的填写详细说明可查看创建与配置第三方平台准备工作。
 关于权限集的详细介绍可查看权限集说明。

## # 权限集配置
 需前往「微信开发者平台 - 第三方平台」进行操作，详情可查看修改权限集。

## # 开发资料配置
 开发资料修改后，只会对「授权测试公众号/小程序列表」中的授权账号生效。
 需全网发布后方可对全网的授权账号生效。
 1）开发配置如下：
  2）所有配置均可修改。
 3）对于符合切换云服务模式条件的账号，可在「开发模式」中进行切换。


#################### extra_get_authorizer_option.md
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
