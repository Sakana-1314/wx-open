====================================================================================================
URL: https://developers.weixin.qq.com/doc/oplatform/en/Third-party_Platforms/Authorization_Process_Technical_Description.html
====================================================================================================
## # Technical Description of the Authorization Process
 The technical implementation of the process of authorizing a Mini Program or Official Account to a third-party platform is relatively simple. We use a Mini Program as an example, as shown below:
  A detailed description is provided below:
 Step 1: The third-party platform obtains the pre-authorization code (pre_auth_code)
 Details
 Step 2: Guide the user to the authorization page
 The third-party platform can place a "Weixin Official Account Authorization" or "Mini Program Authorization" entry on its own website, or generate an authorization link and place it on a mobile website to lead the Official Account or Mini Program admin to the authorization page.
 Method 1: Scan a code on the authorization and registration page to grant authorization
 Authorization page URL:
 https://mp.weixin.qq.com/cgi-bin/componentloginpage?component_appid=xxxx&pre_auth_code=xxxxx&redirect_uri=xxxx&auth_type=xxx.

| Parameter | Required | Description |
|---|---|---|
| component_appid | Yes | Third-party platform's AppID |
| pre_auth_code | Yes | Pre-authorization code |
| redirect_uri | Yes | Callback URI |
| auth_type | No | The type of account to authorize. 1: Only Official Accounts are displayed on the phone after the merchant scans the code; 2: Only Mini Programs are displayed; 3: Both Official Accounts and Mini Programs are displayed. If this parameter is not set, the Official Accounts and Mini Programs are both displayed by default. The third-party platform developers can use this field to control the account types for authorization. |
| biz_appid | No | Specifies the unique Mini Program or Official Account for authorization. |

Method 2: Tap the link in the app for quick authorization
The third-party platform can generate an authorization link that can be given directly to the admin in the app. After the admin provides confirmation, the authorization is granted.

 Authorization link:

```
https://mp.weixin.qq.com/safe/bindcomponent?action=bindcomponent&auth_type=3&no_scan=1&component_appid=xxxx&pre_auth_code=xxxxx&redirect_uri=xxxx&auth_type=xxx&biz_appid=xxxx#wechat_redirect
```


| Parameter | Required | Description |
|---|---|---|
| component_appid | Yes | Third-party platform's AppID |
| pre_auth_code | Yes | Pre-authorization code |
| redirect_uri | Yes | Callback URI |
| auth_type | Yes | The type of account to authorize. 1: Only Official Accounts are displayed on the phone after the merchant taps the link; 2: Only Mini Programs are displayed; 3: Both Official Accounts and Mini Programs are displayed. If this parameter is not set, the Official Accounts and Mini Programs are both displayed by default. The third-party platform developers can use this field to control the account types for authorization. |
| biz_appid | No | Specifies the unique Mini Program or Official Account for authorization. |

Note: The auth_type and biz_appid are mutually exclusive.
 Step 3: The user confirms and agrees to grant login authorization to the third-party platform
 After entering the third-party platform authorization page, the user completes the authorization process by confirming and granting authorization for the their Official Account or Mini Program to the third-party platform.
 Step 4: After authorization, the callback URI is used to obtain the authorization_code and expiration time.
 After the authorization process is completed, the authorization page automatically redirects to the callback URI and the authorization code and expiration time are returned in the URL parameter (redirect_url?auth_code=xxx&expires_in=600).
 Step 5: Use the authorization code to call Official Account or Mini Program APIs
 After obtaining the authorization code, the third-party platform can use it to obtain the credential (authorizer_access_token or "token") for calling Official Account or Mini Program APIs according to the Official Account Development Documentation or Mini Program Development Documentation.
The APIs that can be called are determined by the permission sets the user grants to the third-party platform as well as the Official Account's or Mini Program's native API permissions. The third-party platform can also use the JS SDK and other capabilities. For details, see [Official Account Third-Party Platform API Description].
 Descriptions of each API and mechanism are provided below (note that the caller's IP address must be verified for all API calls, so only IP addresses in the IP whitelist provided when you apply for a third-party platform can be used to call appropriate APIs):

| Feature | API Description |
|---|---|
| 1. Push component_verify_ticket | For security reasons, after the application for creation of a third-party platform is approved, the Weixin server will push a component_verify_ticket to the third-party message receipt address every 10 minutes. The third-party platform can use this API to get the API call credential. |
| 2. Get third-party platform component_access_token | The third-party platform uses its own component_appid (the AppID and AppSecret on the third-party platform details page of the Weixin Open Platform Management Center) and component_appsecret as well as the component_verify_ticket (the security ticket pushed once every 10 minutes) to obtain its API call credential (component_access_token). |
| 3. Get pre_auth_code | The third-party platform uses its API call credential (component_access_token) to get the pre-authorization code (pre_auth_code) used for authorization process preparation. |
| 4. Use the authorization code to obtain the API call credential and authorization information of the Official Account or Mini Program | The third-party platform uses the authorization code and its API call credential (component_access_token) to get the API call credential (authorizer_access_token and authorizer_refresh_token that is used to quickly refresh the former when it expires) and authorization information (e.g., the permissions granted) of the Official Account or Mini Program. |
| 5. Get/Refresh the API call credential of the Official Account or Mini Program | The third-party platform uses the authorizer_refresh_token to refresh the credential for calling the Official Account or Mini Program APIs. |
| 6. Get the basic information of the authorized Official Account or Mini Program | When necessary, the third-party platform can get the basic information of the Official Account or Mini Program, including the account name and account type. |
| 7. Get the option setting information of the authorizer | When necessary, the third-party platform can get the option settings of the Official Account or Mini Program, including the location report settings, voice recognition settings, and multi-customer service settings. |
| 8. Set the authorizer's option information | When necessary, the third-party platform can modify the option settings of the Official Account or Mini Program, including the location report settings, voice recognition settings, and multi-customer service settings. |
| 9. Push authorization-related notifications | When the Official Account or Mini Program grants authorization to the third-party platform, cancels authorization, or updates the authorization, developers are informed of such action by push notification. |
| Next: Call APIs on behalf of the Official Account or Mini Program | After authorization is granted, the third-party platform can use the Official Account or Mini Program API call credential (authorizer_access_token) to call APIs on its behalf. For details, see the "Implementing Businesses on Behalf of Official Accounts" and "Implementing Businesses on Behalf of Mini Programs". |


#### # 1. Push component_verify_ticket protocol
 Details

#### # 2. Get third-party platform component_access_token
 Details

#### # 3. Get pre_auth_code
 Details

#### # 4. Use the authorization code to obtain the API call credential and authorization information of the Official Account or Mini Program
 Details

#### # 5. Get/Refresh the API call credential of the Official Account or Mini Program
 Details

#### # 6. Get the Authorizer's account information
 Details

#### # 7. Get the Authorizer's option information
 Details

#### # 8. Set the Authorizer's option information
 Details

#### # 9. Push notifications of authorization changes
 Details

 The translations are provided by WeChat Translation and are for reference only. In case of any inconsistency and discrepancy between the Chinese version and the English version, the Chinese version shall prevail.Incorrect translation. Tap to report.
