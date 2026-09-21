====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/en/openApi/OpenApiDoc/authorization-management/setAuthorizerOptionInfo.html
====================================================================================================
# # Setting Authorized Party Options Information
  Debugging tools
 Interface should be called on the server side, seeHattori API。

## # Interface Dxplaination

### # Interface name
 setAuthorizerOptionInfo

### # Functional description
 this API Used to set the authorized party's Official Account message template/Mini Program options information, such as: geographical location report, speech recognition switch, multi-customer service switch. If you encounter problems during use, you can use theOpen Platform Service Provider ZonePosting exchanges

### # Note
 Note: To set the options setting information, you need to have the authorization of the authorized party, see the permission set explaination.

## # Calling mode

### # HTTPS call

```
POST https://api.weixin.qq.com/cgi-bin/component/set_authorizer_option?access_token=ACCESS_TOKEN
```


### # Third Party Invocation
 The calling method and parameters are the same as HTTPS, only the calling token is different

### # Request parameters

| attribute | type | Required | Introductions |
|---|---|---|---|
| access_Token | string | yes | Interface invokes the certificate, which is URL Parameters, non Body Parameters. useauthorizer_access_Token |
| option_name | string | yes | Name of option |
| 初始值_value | string | yes | Option value set |


### # Return parameters

| attribute | type | Introductions |
|---|---|---|
| errcode | number | Error code |
| errmsg | string | Error message |


## # Other Notes

### # option_name及option_Dxplaination of value

| option_name | Option Name Dxplaination | option_value | Option Value Dxplaination |
|---|---|---|---|
| location_report | Geographic Reporting Options | 0 | No reporting |
|  |  | 1 | Reporting when entering a session |
|  |  | 2 | Report every 5 seconds. |
| voice_recognize | Speech recognition switch options | 0 | Turn off speech recognition. |
|  |  | 1 | Turn on speech recognition. |
| customer_service | Multiple Customer Service Switch Options | 0 | Close Multiple Customer Service |
|  |  | 1 | Open multiple customer service |


## # Call Example
 Example Dxplaination: HTTPS requests

### # Sample Request Data

```
{
  "option_name": "option_name_value",
  "option_value": "option_value_value"
}
```


### # Return Data Example

```
{
  "errcode": 0,
  "errmsg": "ok"
}
```


### # Error code

| Error code | Error code | Solutions |
|---|---|---|
| 40013 | invalid appid | Illegal AppID , ask the developer to check AppID The correctness of the, avoid unusual characters, pay attention to the case |


 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
