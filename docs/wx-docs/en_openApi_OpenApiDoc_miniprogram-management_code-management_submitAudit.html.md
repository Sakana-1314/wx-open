====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/en/openApi/OpenApiDoc/miniprogram-management/code-management/submitAudit.html
====================================================================================================
# # Submit Code Review
  Debugging tools
 Interface should be called on the server side, seeHattori API。

## # Interface Dxplaination

### # Interface name
 submitAudit

### # Functional description
 After calling commit to upload code for the Mini Program, you can call this interface and submit the uploaded code for review.
 In order to improve the success rate of submitted code review, developers can first readDxplaination of "Pre-Checking Items for Submitting Code Review"
If you encounter problems during use, you canOpen Platform Service Provider ZonePosting exchanges.

### # Note
 If the interface involves a geolocation-dependent privacy interface, you need to configure requiredPrivateInfos in the ext.json parameter of the interface where the code is committed
 After uploading the code, you need to wait until the detection task is over before submitting the code audit, otherwise, 61039 errors will occur when submitting the code audit.
 Developers can do this throughgetCodePrivacyInfoInterface to get the test results, confirm the end of the test task and then submit the code review
 Do not retry the upload code and submit code audit interfaces together, or 61039 errors will still occur if the detection task after each upload is not over and a new detection task is initiated.
 If the location-dependent interface is used in the code, but is not yet available in ext_Configure requiredPrivateInfos in the json parameter. A 61040 error will occur when the code is submitted for review.
 If the code uses a geolocation interface, but the appid Mini Program does not have permission for the geolocation api, then a 61040 error will appear when the code is submitted for review.
 If it is detected that the code contains a privacy-related interface, but the developer confirms that it is not used, you can use the privacy_api_not_Use to make a statement

## # Calling mode

### # HTTPS call

```
POST https://api.weixin.qq.com/wxa/submit_audit?access_token=ACCESS_TOKEN
```


### # Third Party Invocation
 The calling method and parameters are the same as HTTPS, only the calling token is different

 The permission set id to which this interface belongs is: 18

 After the service provider has been authorized with one of the permission sets, it can do so by usingauthorizer_access_TokenCalling on behalf of the merchant

### # Request parameters

|  | attribute | type | Required | Introductions |
|---|---|---|---|---|
|  | access_Token | string | yes | Interface invokes the certificate, which is URL Parameters, non Body Parameters. useauthorizer_access_Token |
|  | item_list | array | yes | List of audit items (optional, at most 5 Item)The category is required and must be filled in the category that has been configured in the Mini Program. |
|  |  | attribute | type | Required | Introductions |
|  | address | string | no | The page of the Mini Program, which can be obtained through the "get a list of pages of the Mini Program getCodePage" interface |
|  | tag | string | no | Mini Program labels, separated by spaces, at most 10 Label length up to 20 |
|  | first_class | string | yes | First-level category name, available through the getAllCategoryName interface |
|  | second_class | string | yes | Secondary category name, available through the getAllCategoryName interface |
|  | third_class | string | no | Third-level category name, available through the getAllCategoryName interface |
|  | title | string | no | The title of the Mini Program page, at most 32 |
|  | first_id | number | yes | First-level category id, available through the getAllCategoryName interface |
|  | second_id | number | yes | Secondary category id, available through the getAllCategoryName interface |
|  | third_id | number | no | Class III id, available through the getAllCategoryName interface |

  feedback_info string no Feedback, at most 200 word
  feedback_stuff string no use | Divided media_id List, at most 5 The picture, Can be accessed throughNew temporary material interfaceUpload and get
  version_desc string no Mini Program Version Notes and Functional Explanation
  preview_info object no Preview information (Mini Program page screenshot and operation record screen)

|  | attribute | type | Required | Introductions |
|---|---|---|---|---|
|  | video_id_list | array | yes | Video screen mediaid list, available through the arraignment material upload interface |
|  | pic_id_list | array | yes | Screenshot mediaid list, available through the arraignment material upload interface |

  ugc_declare object no User-Generated Content Scenario (UGC) information security statement

|  | attribute | type | Required | Introductions |
|---|---|---|---|---|
|  | scene | array | no | UGC Scene 0, does not involve user-generated content, 1. User profile, 2. Picture, 3. Video, 4. Text, 5 Audio, You can choose more than one. You do not need to fill in the following fields when you fill in 0 |
|  | method | array | no | Content security mechanism 1. Use the content security API recommended by the platform, 2. Use other content audit products, 3. Pass manual audit checks, 4. Do not do content audit checks |
|  | other_scene_desc | string | no | When selecting other instructions, do not timeout 256 words |
|  | has_audit_team | number | no | Is there an audit team, 0. None, 1. Yes, default 0 |
|  | audit_desc | string | no | Describe the current review mechanism for UGC content, no more than 256 words |

  privacy_api_not_use boolean no Used to declare whether or not to use "privacy-related interfaces detected in code but not configured"
  order_path string no Order Center path

### # Return parameters

| attribute | type | Introductions |
|---|---|---|
| errcode | number | Return code |
| errmsg | string | Error message |
| auditid | number | Audit Number |


## # Other Notes

### # Additional Notes
 Only if the previous version is rejected can it be used feedback_info、feedback_stuff These two fields, otherwise ignore processing.
 When the Mini Program is submitted for review for the first time and the category includes social - community/Forum, Social - Notes, Community - Q & A One of these needs to be filled ugc_declare
 Can be called additionallyUpload Arraignment MaterialInterface, the Mini Program page screenshot and operation recording screen upload, arraignment with the relevant parameters, can help the auditor to determine.

### # Code Review Results
 When the mini program has the audit results, the WeChat server will receive messages and events from the third-party platform. The URL (filled in when creating a third-party platform) to POST To push relevant notifications
 receive POST Request, simply return the character string success. To enhance security, postdata to hit the target xml Will use encryption and decryption at the time of the service request key To encrypt, seeEncryption and Decryption Technology Program, Decryption is required after receiving the push (seeMessage Encryption and Decryption Access Guide）,
 In addition to message notifications, third-party platforms are also available via the interfaceQuery the audit status of the specified version、Check the status of the latest submission

### # Field Dxplaination

| parameter | type | Introductions |
|---|---|---|
| ToUserName | String | The Original Mini Program ID |
| FromUserName | String | The sender account number (a OpenID, where the sender is the system account) |
| CreateTime | Number | Message creation time (int), timestamp |
| MsgType | String | Message type event |
| Event | String | Type of event |
| SuccTime | Number | Time stamp when audit is successful |
| FailTime | Number | The time stamp that the audit failed |
| DelayTime | Number | Timestamp when audit is delayed |
| Reason | String | Reasons for Failure to Audit |
| ScreenShot | String | Example of screenshot that did not pass the review. use | Segregated media_id Of the list, which is available through theGet permanent material interfacePull Screenshot Content |


#### # Type of event

| Type of event | Introductions |
|---|---|
| weapp_audit_success | Approved by |
| weapp_audit_fail | Audit not passed |
| weapp_audit_delay | Deferred audit |

Example after the push content is decrypted:

#### # Approved by

```
<xml>
  <ToUserName><![CDATA[gh_FB9688c2a4b2]]></ToUserName>
  <FromUserName><![CDATA[od1P50M-fNQI5Gcq-trm4a7apsU8]]></FromUserName>
  <CreateTime>1488856741</CreateTime>
  <MsgType><![CDATA[event]]></MsgType>
  <Event><![CDATA[weapp_audit_success]]></Event>
  <SuccTime>1488856741</SuccTime>
</xml>
```


#### # Audit not passed

```
<xml>
  <ToUserName><![CDATA[gh_FB9688c2a4b2]]></ToUserName>
  <FromUserName><![CDATA[od1P50M-fNQI5Gcq-trm4a7apsU8]]></FromUserName>
  <CreateTime>1488856591</CreateTime>
  <MsgType><![CDATA[event]]></MsgType>
  <Event><![CDATA[weapp_audit_fail]]></Event>
  <Reason><![CDATA[1: Account information does not meet the norms:<br>(1): Contains pornographic elements<br>2: Service category " Finance - Insurance_" Not consistent with the feature page you set when you submitted your code for review:<br>(1): Some of the tabs set by the feature page do not belong to the selected service category.<br>(2): Some of the tags in the feature page settings are not relevant to the content of that page.<br>]]></Reason>
  <FailTime>1488856591</FailTime>
  <ScreenShot>xxx|yyy|zzz</ScreenShot>
</xml>
```


#### # Deferred audit

```
<xml>
  <ToUserName><![CDATA[gh_FB9688c2a4b2]]></ToUserName>
  <FromUserName><![CDATA[od1P50M-fNQI5Gcq-trm4a7apsU8]]></FromUserName>
  <CreateTime>1488856591</CreateTime>
  <MsgType><![CDATA[event]]></MsgType>
  <Event><![CDATA[weapp_audit_delay]]></Event>
  <Reason><![CDATA[In order to better serve the Mini Program, your service provider is optimizing the arraignment system, which may lead to the growth of the audit time limit, please be patient.]]></Reason>
  <DelayTime>1488856591</DelayTime>
</xml>
```


## # Call Example
 Example Dxplaination: HTTPS requests

### # 初始值

```
{
	"item_list": [
	{
		"address":"index",
		"tag": "Learn life " ,
		"first_class": "entertainment,"
		second_class": "Information,"
		"first_id":1,
		second_id":2,
		"title": "Home"
	}
	{
		"address":"page/logs/logs",
		"tag": "Learn work " ,
		"first_class": "education,"
		second_class": "academic education."
		"third_class": "High,"
		"first_id":3,
		second_id":4,
		"third_id":5,
		"title": "Log"
	}
	],
	"feedback_info": "blablabla",
    "feedback_stuff": "xx|yy|zz",
    "preview_info" : {
        " video_id_list": ["xxxx"],
        "pic_id_list": ["xxxx", "yyyy", "zzzz" ]
    },
    "version_desc":"bla"
    "ugc_declare": {
        "scene": [
            1,
            2
        ],
        "method": [
            1
        ],
        "has_audit_team": 1,
        "audit_Desc": "bla"
    }
}
```


### # Return Data Example

```
{
  "errcode": 0,
  "errmsg": "ok",
  "auditid": 1234567
初始值
```


### # Error code

| Error code | Error code | Solutions |
|---|---|---|
| 0 | ok | ok |
| 86000 | should be called only from third party | Not called by a third-party Mini Program |
| 86001 | component experience version not exists | There is no code submitted by third parties. |
| 85006 | Label format error | Label format error |
| 85007 | Page Path Error | Page Path Error |
| 85008 | category is in invalid format | The current Mini Program does not have a category that has been approved, please add a category and try again after success. |
| 85009 | already submit a version Under auditing | There is already a version under review |
| 85010 | item_list Project is empty. | item_list Project is empty. |
| 85011 | Error in title | Error in title |
| 85023 | item size is not in valid range | The number of items filled in the audit list is not 1-5 within |
| 85077 | Mini Program Category Information Invalid (Category contains the official list, please re-select the category) | Mini Program Category Information Invalid (Category contains the official list, please re-select the category) |
| 86002 | miniprogram have not completed init procedure | The Mini Program has not yet set a nickname, avatar, or profile. Please set up and resubmit |
| 85085 | submit audit reach limit pleasetry later | The number of Mini Program arraignments has reached the limit of this month, please click on the "Self-Help Temporary Application Quota" |
| 85086 | must commit before submit audit | Upload code in advance before submitting code for review |
| 85087 | Mini Program is used api NavigateToMiniProgram, please declare the jump appid Resubmit after the list | Mini Program is used api NavigateToMiniProgram, please declare the jump appid Resubmit after the list |
| 87006 | this is game miniprogram submitaudit is forbidden | Small games cannot be submitted |
| 86007 | Small Business Prohibited | Small Business Prohibited |
| 85051 | data too large | version_Desc or preview_Info overlimit |
| 85092 | invalid preview_info format | preview_Info format error |
| 85093 | preview_info Number of videos or pictures exceeded | preview_info Number of videos or pictures exceeded |
| 85094 | need add ugc declare | Need to provide information on audit mechanism |
| 86009 | Service provider's ability to add Mini Program code arraignment is limited | Service provider's ability to add Mini Program code arraignment is limited |
| 86010 | Service provider iteration Mini Program code arraignment ability is limited | Service provider iteration Mini Program code arraignment ability is limited |
| 9400001 | The development of the Mini Program has opened the Mini Program live broadcast permissions, does not support the release version. If you need to release version, please unbind the development of Mini programs before operation. | The development of the Mini Program has opened the Mini Program live broadcast permissions, does not support the release version. If you need to release version, please unbind the development of Mini programs before operation. |
| 9402202 | concurrent limit | Do not submit frequently until the last operation is completed. |
| 61040 | " The privacy interface xxx configured by ext.json does not have permission, please apply for permission before submitting for review. Or code contains ext.json does not configure the privacy interface xxx(No permission required), Configure and request permission or commit not to use these interfaces (set parameter privacy_api_not_Use is true) before submitting it for review. |  |
|  |  |  |


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
