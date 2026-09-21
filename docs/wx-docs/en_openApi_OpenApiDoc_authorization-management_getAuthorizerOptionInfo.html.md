====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/en/openApi/OpenApiDoc/authorization-management/getAuthorizerOptionInfo.html
====================================================================================================
# # Get Authorized Party Options Information
  Debugging tools
 Interface should be called on the server side, seeHattori API。

## # Interface Dxplaination

### # Interface name
 getAuthorizerOptionInfo

### # Functional description
 this API For obtaining the authorization party's Official Account message template/Options for setting information such as: Location Report, Speech recognition switch, Multi-Client switch. If you encounter problems during use, you canOpen Platform Service Provider ZonePosting exchanges

## # Calling mode

### # HTTPS call

```
POST https://api.weixin.qq.com/cgi-bin/component/get_authorizer_option?access_token=ACCESS_TOKEN
```


### # Third Party Invocation
 The calling method and parameters are the same as HTTPS, only the calling token is different

### # Request parameters

| attribute | type | Required | Introductions |
|---|---|---|---|
| access_Token | string | yes | Interface invokes the certificate, which is URL Parameters, non Body Parameters. useauthorizer_access_Token |
| option_name | string | yes | Name of option |


### # Return parameters

| attribute | type | Introductions |
|---|---|---|
| option_name | string | Name of option |
| option_value | 初始值 | Option Value |


## # Call Example
 Example Dxplaination: HTTPS calls

### # Sample Request Data

```
{
  "option_name": "option_name_value"
}
```


### # 初始值

```
{
  "option_name": "voice_recognize",
  "option_value": "1"
}
```


### # Error code

| Error code | 初始值 | Solutions |
|---|---|---|
| 40001 | invalid credential access_Token isinvalid or not 初始值 | Obtain access_Token time AppSecret Error, or access_Token Invalid. Please take the developer more seriously. AppSecret Of the correctness, or to see if you are working for the appropriate Official Account message template Call interface |
| 0 | ok | ok |


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
