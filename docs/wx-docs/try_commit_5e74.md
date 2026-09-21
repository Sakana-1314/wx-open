====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/Third-party_Platforms/2.0/api/code/commit.html
====================================================================================================
## # 上传小程序代码并生成体验版
 第三方平台需要先将草稿添加到代码模板库，或者从代码模板库中选取某个代码模板，得到对应的模板 id（template_id）； 然后调用本接口可以为已授权的小程序上传代码。
 注意，通过该接口提交代码后，会同时生成体验版。
 使用过程中如遇到问题，可在开放平台服务商专区发帖交流。

### # 请求地址

```
POST https://api.weixin.qq.com/wxa/commit?access_token=ACCESS_TOKEN
```


### # 请求参数说明

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| access_token | String | 是 | 第三方平台接口调用令牌authorizer_access_token |
| template_id | String | 是 | 代码库中的代码模板 ID，可通过获取代码模板列表接口获取template_id 注意，如果该模板id为标准模板库的模板id，则ext_json可支持的参数为：{"extAppid":" ", "ext": {}, "window": {}} |
| ext_json | String | 是 | 为了方便第三方平台的开发者引入 extAppid 的开发调试工作，引入ext.json配置文件概念，该参数则是用于控制ext.json配置文件的内容。关于该参数的补充说明请查看下方的"ext_json补充说明"。 |
| user_version | String | 是 | 代码版本号，开发者可自定义（长度不要超过 64 个字符） |
| user_desc | String | 是 | 代码描述，开发者可自定义 |

POST 数据示例:
 普通模板库代码提交的示例

```
{
  "template_id": "0",
  "ext_json": "{\"extAppid\":\"\",\"ext\":{\"attr1\":\"value1\",\"attr2\":\"value2\"},\"extPages\":{\"index\":{},\"search/index\":{}},\"pages\":[\"index\",\"search/index\"],\"window\":{},\"networkTimeout\":{},\"tabBar\":{},\"plugin\":{}}",
  "user_version": "V1.0",
  "user_desc": "test"
}
```

标准模板库代码提交的示例

```
{
  "template_id": "0",
  "ext_json": "{\"extAppid\":\"\",\"ext\":{\"attr1\":\"value1\",\"attr2\":\"value2\"},\"window\":{}}",
  "user_version": "V1.0",
  "user_desc": "test"
}
```


### # ext_json补充说明
 为了便于第三方平台使用同一个小程序模板为不同的小程序提供服务，第三方可以将自定义信息放置在 ext_json 中，在模板小程序中，可以使用 wx.getExtConfigSync 接口获取自定义信息，从而区分不同的小程序。详见：小程序模板开发
 ext_json 中的参数可选，参数详见小程序配置;但是，如果是模板id为标准模板库的模板id，则ext_json可支持的参数为：{"extAppid":'', "ext": {}, "window": {}}
 ext_json 中有限支持 pages，支持配置模板页面的子集（ext_json 中不可新增页面）。
 ext_json 中有限支持 subPackages，支持配置模板分包及其页面的子集（ext_json 中配置的分包必须已声明于模板中，且不可新增分包页面）。
 ext_json支持plugins配置，该配置会覆盖模板中的app.json中的plugins配置。关于plugin的使用详情请参考使用插件。
 如果代码中已经有配置，则配置的合并规则为：
 ext整体替换
 pages整体替换
 extPages中找到对应页面，同级覆盖page.json
 window同级覆盖
 extAppid直接加到app.json
 networkTimeout同级覆盖
 customOpen整体替换
 tabbar同级覆盖
 functionPages整体替换
 subPackages整体替换
 navigateToMiniProgaramAppIdList：整体替换
 plugins整体替换
 补充说明：
 什么叫同级覆盖？以『window同级覆盖』为例，遍历extjson的window对象成员，如果app.json的window对象存在该对象成员，则覆盖，如果不存在，就添加。
 什么叫整体替换？以『plugins整体替换』为例，如果app.json存在plugin对象，就用extjson里的plugin对象覆盖，如果不存在，就添加。
 特殊字段说明：

| 参数 | 说明 |
|---|---|
| ext | 自定义字段仅允许在这里定义，可在小程序中调用 |
| extPages | 页面配置 |
| extAppid | 授权方 Appid，可填入商户 AppID，以区分不同商户 |
| sitemap | 用于配置小程序及其页面是否允许被微信索引 |


### # 返回参数说明

| 参数 | 类型 | 说明 |
|---|---|---|
| errcode | number | 返回码 |
| errmsg | string | 错误信息 |

返回结果示例：

```
{
  "errcode": 0,
  "errmsg": "ok"
}
```


### # 返回码说明

| 返回码 | 说明 |
|---|---|
| -1 | 系统繁忙 |
| 85013 | 无效的自定义配置 |
| 85014 | 无效的模板编号 |
| 85043 | 模板错误 |
| 85044 | 代码包超过大小限制 |
| 85045 | ext_json 有不存在的路径 |
| 85046 | tabBar 中缺少 path |
| 85047 | pages 字段为空 |
| 85048 | ext_json 解析失败 |
| 80082 | 没有权限使用该插件 |
| 80067 | 找不到使用的插件 |
| 80066 | 非法的插件版本 |
| 9402202 | 请勿频繁提交，待上一次操作完成后再提交 |
| 9402203 | 标准模板ext_json错误，传了不合法的参数， 如果是标准模板库的模板，则ext_json支持的参数仅为{"extAppid":'', "ext": {}, "window": {}} |
| 其他错误码 | 请查看全局错误码 |


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
