====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/openApi/miniprogram-management/code-management/api_commit.html
====================================================================================================
# # 上传代码并生成体验版
  调试诊断
 接口应在服务器端调用，不可在前端（小程序、网页、APP等）直接调用，具体可参考接口调用指南。
 接口英文名：commit
 本接口用于上传第三方代开发小程序的代码并生成体验版。具体使用流程为：第三方平台需要先通过addToTemplate接口将草稿添加到代码模板库，然后从代码模板库中通过getTemplateList接口获取对应的模板 id（template_id），然后调用本接口可以为已授权的小程序上传代码并生成体验版。
 更多关于第三方平台代开发小程序模板库管理查看文档：小程序模板库

## # 1. 调用方式

### # HTTPS 调用

```
POST https://api.weixin.qq.com/wxa/commit?access_token=ACCESS_TOKEN
```


### # 云调用
 本接口不支持云调用。

### # 第三方调用
 本接口支持第三方平台代商家调用。

 该接口所属的权限集 id 为：18

 服务商获得其中之一权限集授权后，可通过使用 authorizer_access_token 代商家进行调用，具体可查看 第三方调用 说明文档。

## # 2. 请求参数

### # 查询参数 Query String Parameters

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| access_token | string | 是 | 接口调用凭证，可使用 authorizer_access_token |

### # 请求体 Request Payload

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| template_id | number | 是 | 代码库中的代码模板 ID，可通过getTemplateList接口获取代码模板template_id。 |
| ext_json | string | 是 | 为了方便第三方平台的开发者引入 extAppid 的开发调试工作，引入ext.json配置文件概念，该参数则是用于控制ext.json配置文件的内容。关于该参数的补充说明请查看下方的"ext_json补充说明"。 |
| user_version | string | 是 | 代码版本号，开发者可自定义（长度不要超过 64 个字符） |
| user_desc | string | 是 | 代码描述，开发者可自定义 |


## # 3. 返回参数

### # 返回体 Response Payload

| 参数名 | 类型 | 说明 |
|---|---|---|
| errcode | number | 返回码 |
| errmsg | string | 返回信息 |


## # 4. 注意事项
 如果接口中涉及地理位置相关隐私接口，需要在ext_json参数中配置requiredPrivateInfos
 上传代码后，需要等检测任务结束后方可提交代码审核，否则提交代码审核时会出现61039报错
 开发者可以通过getCodePrivacyInfo接口获取检测结果，确认检测任务结束后再提交代码审核
 不要将上传代码和提交代码审核两个接口一起重试，否则每次上传代码后的检测任务未结束又发起新的检测任务，依旧会出现61039报错。
 如果代码里使用了地理位置相关接口，但是又尚未在ext_json参数中配置requiredPrivateInfos。那么提交代码审核时会出现61040报错。
 如果代码里使用了地理位置相关接口，但是该小程序appid又未获得对应地理位置api的权限，那么提交代码审核时会出现61040报错。如果开发者确认不需要使用地理位置相关接口，那么不需要去申请权限，只需要通过提交代码审核接口中的privacy_api_not_use参数进行声明不使用即可

# # 其他说明

## # 关于新增requiredPrivateInfos说明
 关于地理位置接口新增与相关流程调整可以查看社区公告：https://developers.weixin.qq.com/community/develop/doc/000a02f2c5026891650e7f40351c01，7.14后，在代码中使用的地理位置相关接口（共计 8 个，见表1），第三方开发者均需要在 ext_json 参数中 requiredPrivateInfos 配置项中声明
 在ext_json参数中配置requiredPrivateInfos，其规则为「整体替换」。即如果在app.json里也配置了，那么最终会是ext_json的配置会覆盖app.json配置的requiredPrivateInfos。其余规则可查看下方的「ext_json补充说明」
 在ext_json参数中配置requiredPrivateInfos示例如下

```
{
  "template_id": "95",
  "ext_json": "{\"requiredPrivateInfos\":[\"onLocationChange\",\"startLocationUpdate\"]}",
  "user_version": "V1.0",
  "user_desc": "test"
}
```

requiredPrivateInfos主要会检查格式是否正确，填入的api名称是否正确，填入的api名称是否有权限，填入的api名称是否互斥。对应的错误码可查看文档末尾的错误码文档。
 requiredPrivateInfos在2022.7.14后才会生效，文档提前更新是为了方便开发者可以提前了解接口的参数变更规则，提前进行调整。

### # ext_json补充说明
 为了便于第三方平台使用同一个小程序模板为不同的小程序提供服务，第三方可以将自定义信息放置在 ext_json 中，在模板小程序中，可以使用 wx.getExtConfigSync 接口获取自定义信息，从而区分不同的小程序。详见：小程序模板开发
 ext_json 中的参数可选，参数详见小程序配置;
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


## # 5. 代码示例
 请求示例

```
{
  "template_id": "0",
  "ext_json": "{\"extAppid\":\"\",\"ext\":{\"attr1\":\"value1\",\"attr2\":\"value2\"},\"extPages\":{\"index\":{},\"search/index\":{}},\"pages\":[\"index\",\"search/index\"],\"window\":{},\"networkTimeout\":{},\"tabBar\":{},\"plugin\":{}}",
  "user_version": "V1.0",
  "user_desc": "test"
}
```

返回示例

```
{
  "errcode": 0,
  "errmsg": "ok"
}
```


## # 6. 错误码
 以下是本接口的错误码列表，其他错误码可参考 通用错误码；调用接口遇到报错，可使用官方提供的 API 诊断工具 辅助定位和分析问题。

| 错误码 | 错误描述 | 解决方案 |
|---|---|---|
| -60005 |  |  |
| -1 | system error | 系统繁忙，此时请开发者稍候再试 |
| 40001 | invalid credential access_token isinvalid or not latest | 获取 access_token 时 AppSecret 错误，或者 access_token 无效。请开发者认真比对 AppSecret 的正确性，或查看是否正在为恰当的公众号调用接口 |
| 40014 | invalid access_token | 不合法的 access_token ，请开发者认真比对 access_token 的有效性（如是否过期），或查看是否正在为恰当的公众号调用接口 |
| 80066 |  | 非法的插件版本 |
| 80067 |  | 找不到使用的插件 |
| 80082 |  | 没有权限使用该插件 |
| 85013 | invalid ext_json, parse fail or containing invalid path | 无效的自定义配置 |
| 85014 | template not exist | 无效的模板编号 |
| 85043 | invalid template, something wrong? | 模板错误 |
| 85044 | package exceed max limit | 代码包超过大小限制 |
| 85045 | some path in ext_json not exist | ext_json 有不存在的路径 |
| 85046 | pagepath missing in tabbar list | tabBar 中缺少 path |
| 85047 | pages are empty | pages 字段为空 |
| 85048 | parse ext_json fail | ext_json 解析失败 |
| 85310 | is wrong API name | requiredPrivateInfos 的格式有问题或者api名称写错了 |
| 85311 | is mutually exclusive | requiredPrivateInfos包含了互斥的api |
| 85312 | is not authorized | requiredPrivateInfos 配置了没有权限的api |
| 9402202 | concurrent limit | 请勿频繁提交，待上一次操作完成后再提交 |
| 9402203 | 标准模板extjson错误 | 标准模板ext_json错误，传了不合法的参数， 如果是标准模板库的模板，则ext_json支持的参数仅为{"extAppid":'', "ext": {}, "window": {}} |


## # 7. 适用范围
 本接口支持「第三方平台」账号类型代调用，权限集请参考「调用方式」部分。其他账号类型如无特殊说明，均不可调用。

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
