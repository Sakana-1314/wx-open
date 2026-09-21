package model

import "fmt"

// errcodeRule 一条返回码处置规则。
type errcodeRule struct {
	Class ErrorClass
	Text  string
	Hint  string
}

// errcodeRules 微信返回码处置表。
//
// 分类语义（与作业引擎的重试策略一一对应）：
//   - retryable     同一请求退避重试即可；
//   - token_expired 刷新令牌后重试一次；
//   - rate_limited  频次/额度受限，退避或暂停整个作业（提审与加急额度为服务商级、旗下小程序共用）；
//   - environment   IP 白名单等环境问题，需人工在开放平台后台修复，不消耗重试次数；
//   - permanent     永久失败，重试无意义，必须改配置 / 权限 / 请求体。
//
// 代码内不做「未知码默认可重试」的兜底：未知码归入 wechat_unknown 且只重试一次，
// 避免把 9402203 这类永久失败误判成可重试。
var errcodeRules = map[int]errcodeRule{
	-1:      {ClassRetryable, "系统繁忙，请稍后重试", "退避后重试"},
	0:       {ClassPermanent, "请求成功", ""},
	40001:   {ClassTokenExpired, "access_token 无效或不最新", "刷新令牌后重试；确认用的是正确的令牌类型（component 接口不能用 authorizer 令牌，否则 61014）"},
	40013:   {ClassPermanent, "不合法的 appid", "核对 AppID 正确性与大小写"},
	40014:   {ClassTokenExpired, "不合法的 access_token", "刷新令牌后重试"},
	40125:   {ClassPermanent, "无效的 appsecret", "在开放平台后台核对或重置 AppSecret（重置后旧值立即失效）"},
	41001:   {ClassPermanent, "缺少 access_token 参数", "修正请求构造"},
	41002:   {ClassPermanent, "缺少 appid 参数", "修正请求构造"},
	41004:   {ClassPermanent, "缺少 secret 参数", "修正请求构造"},
	41018:   {ClassPermanent, "缺少 component_appid 参数", "修正请求构造"},
	42001:   {ClassTokenExpired, "access_token 已过期", "刷新令牌后重试"},
	42002:   {ClassTokenExpired, "refresh_token 已过期", "需要商家重新授权"},
	42003:   {ClassTokenExpired, "授权码已过期", "重新走授权流程获取授权码"},
	42006:   {ClassTokenExpired, "component_access_token 已过期", "用最近可用的 component_verify_ticket 重新获取"},
	42007:   {ClassTokenExpired, "用户修改密码导致令牌失效", "必须让商家重新授权，无法自动恢复"},
	43001:   {ClassPermanent, "需要 GET 请求", "改用 GET（gettemplatelist / get_latest_auditstatus / undocodeaudit / queryquota / revertcoderelease 均为 GET）"},
	43002:   {ClassPermanent, "需要 POST 请求", "改用 POST"},
	43003:   {ClassPermanent, "需要 HTTPS 请求", "改用 https"},
	44002:   {ClassPermanent, "empty post data", "该接口必须传空 JSON {}（release / getversioninfo / getvisitstatus / get_effective_domain 等）"},
	45002:   {ClassPermanent, "消息内容超过限制", "裁剪内容"},
	45009:   {ClassRateLimited, "接口调用超过天级别频率限制", "可调用 clear_quota 恢复额度（每账号每月共 10 次机会），或次日重试"},
	45011:   {ClassRateLimited, "API 调用太频繁，需降速", "退避到下一分钟重试"},
	45035:   {ClassEnvironment, "来源 IP 不在白名单", "把出口 IP 加入第三方平台 IP 白名单（仅支持具体 IP，最多 100 个）"},
	47001:   {ClassPermanent, "解析 JSON/XML 内容错误", "检查请求体字段与转义（ext_json 必须是 JSON 字符串）"},
	48001:   {ClassPermanent, "api 功能未授权", "该小程序账号自身没有此接口权限，需商家在小程序侧开通"},
	48004:   {ClassPermanent, "api 接口被封禁", "登录小程序后台查看封禁原因"},
	48006:   {ClassRateLimited, "清零次数已达上限", "本月无清零机会，等次日或下月"},
	50002:   {ClassPermanent, "用户受限", "账号被冻结或注销，接口无法解决"},
	61004:   {ClassEnvironment, "调用来源 IP 未注册（第三方平台 IP 白名单）", "把服务器出口 IP 加入第三方平台 IP 白名单后重试"},
	61005:   {ClassTokenExpired, "component ticket 已过期", "使用 12 小时内最近可用的 ticket；必要时调 api_start_push_ticket 恢复推送"},
	61006:   {ClassTokenExpired, "component ticket 无效", "检查 ticket 来源，使用最近可用的有效 ticket"},
	61007:   {ClassPermanent, "该接口权限集未授权给第三方平台", "让商家重新授权并勾选对应权限集（代码管理需要权限集 18）"},
	61011:   {ClassPermanent, "invalid component", "核对 component_appid 与配置"},
	61014:   {ClassPermanent, "必须用 component_access_token 调用该接口", "改用 component_access_token（模板库接口属于此类）"},
	61016:   {ClassPermanent, "功能类目需在第三方平台后台确认", "在后台确认功能类目"},
	61024:   {ClassPermanent, "必须用 api_component_token 获取令牌", "改用官方令牌接口"},
	61028:   {ClassPermanent, "第三方平台未发布", "先完成全网发布"},
	61039:   {ClassRetryable, "隐私接口检查任务未完成", "等待约 1 分钟后重试；提交代码后需等检测任务结束才能提审"},
	61040:   {ClassPermanent, "ext.json 隐私接口未配置或无权限", "在 ext_json 配置 requiredPrivateInfos 并申请权限，或声明 privacy_api_not_use"},
	80066:   {ClassPermanent, "非法的插件版本", "检查 ext_json 中插件版本号"},
	80067:   {ClassPermanent, "找不到使用的插件", "先在小程序后台添加插件"},
	80082:   {ClassPermanent, "没有权限使用该插件", "该小程序无插件使用权限"},
	85008:   {ClassPermanent, "当前小程序没有已审核通过的类目", "先在小程序侧添加类目并等审核通过（类目字段取自 getAllCategoryName）"},
	85009:   {ClassPermanent, "已有正在审核的版本", "等审核完成或先撤回审核"},
	85010:   {ClassPermanent, "item_list 有项目为空", "补齐提审项"},
	85011:   {ClassPermanent, "标题填写错误", "标题需 ≤32 字"},
	85012:   {ClassPermanent, "无效的审核 id", "先查最新审核单获取 auditid"},
	85013:   {ClassPermanent, "无效的自定义配置", "ext_json 需为正确转义的 JSON 字符串，且路径必须存在于模板"},
	85014:   {ClassPermanent, "无效的模板编号", "先同步模板列表，确认 template_id 存在"},
	85015:   {ClassPermanent, "该账号不是小程序账号", "确认代操作对象是小程序"},
	85016:   {ClassPermanent, "域名数量超出上限", "精简域名列表"},
	85017:   {ClassPermanent, "域名输入为空或未在第三方平台添加", "先在第三方平台登记域名（modify_wxa_server_domain）"},
	85018:   {ClassPermanent, "域名未在第三方平台设置", "先在第三方平台登记服务器域名"},
	85019:   {ClassPermanent, "没有审核版本", "先上传代码并提审通过"},
	85020:   {ClassPermanent, "审核状态未满足发布条件", "按 上传→提审→审核通过→发布 的顺序操作"},
	85021:   {ClassPermanent, "审核状态未满足发布条件", "按状态机顺序操作"},
	85023:   {ClassPermanent, "审核列表项目数不在 1-5 以内", "调整 item_list 数量"},
	85043:   {ClassPermanent, "模板错误", "检查模板与 ext_json 是否匹配"},
	85044:   {ClassPermanent, "代码包超过大小限制", "精简代码包（代开发小程序总包 ≤20M，单包/主包 ≤2M）"},
	85045:   {ClassPermanent, "ext_json 中有不存在的路径", "ext_json 里的 pages/extPages 必须存在于模板"},
	85046:   {ClassPermanent, "tabBar 中缺少 path", "补齐 tabBar.path"},
	85047:   {ClassPermanent, "pages 字段为空", "pages 至少包含一个页面"},
	85048:   {ClassPermanent, "ext_json 解析失败", "ext_json 必须是转义后的 JSON 字符串"},
	85051:   {ClassPermanent, "提交数据过大", "version_desc 或 preview_info 超限"},
	85064:   {ClassPermanent, "找不到模板", "同步模板库后重试"},
	85065:   {ClassPermanent, "模板库已满（上限 200）", "先删除不再使用的模板再添加"},
	85077:   {ClassPermanent, "小程序类目信息失效", "类目含官方下架类目，重新选择类目"},
	85079:   {ClassPermanent, "小程序没有线上版本", "先发布一个线上版本再进行灰度"},
	85080:   {ClassPermanent, "提交的审核未通过", "审核通过后才能发布"},
	85081:   {ClassPermanent, "无效的灰度比例", "灰度百分比需为 0-100 的整数"},
	85082:   {ClassPermanent, "灰度比例过低", "灰度比例只能递增"},
	85085:   {ClassRateLimited, "提审数量已达本月上限（quota 用尽）", "在「小程序服务商助手」申请临时额度；额度为服务商级、旗下小程序共用"},
	85086:   {ClassPermanent, "提审前必须先上传代码", "先执行 commit 作业"},
	85087:   {ClassPermanent, "使用了 navigateToMiniProgram 未声明", "在 ext_json 声明跳转 appid 列表"},
	85092:   {ClassPermanent, "preview_info 格式错误", "检查截图/录屏 mediaid"},
	85093:   {ClassPermanent, "preview_info 数量超限", "截图与录屏数量需符合限制"},
	85094:   {ClassPermanent, "需要补充 UGC 审核机制声明", "在提审配置里填写 ugc_declare"},
	85301:   {ClassPermanent, "存在不符合域名规则的域名", "域名需 ICP 备案且不能是 IP / api.weixin.qq.com"},
	85302:   {ClassPermanent, "存在缺少 ICP 备案的域名", "先完成 ICP 备案"},
	85303:   {ClassPermanent, "同时存在域名规则与备案问题", "修正域名格式与备案"},
	85310:   {ClassPermanent, "requiredPrivateInfos 格式或 api 名错误", "核对 8 个地理位置接口名"},
	85311:   {ClassPermanent, "requiredPrivateInfos 含互斥 api", "去掉互斥项"},
	85312:   {ClassPermanent, "requiredPrivateInfos 配置了无权限的 api", "先申请该接口权限"},
	86000:   {ClassPermanent, "该接口只能由第三方平台代调用", "确认使用 authorizer_access_token 代调用"},
	86001:   {ClassPermanent, "不存在第三方已提交的代码", "先执行 commit"},
	86002:   {ClassPermanent, "小程序未完成初始化（昵称/头像/简介）", "先在小程序侧补全基本信息"},
	86007:   {ClassPermanent, "小程序禁止提交", "账号被限制，联系微信"},
	86009:   {ClassPermanent, "服务商新增小程序代码提审能力被限制", "服务商资质问题，联系微信"},
	86010:   {ClassPermanent, "服务商迭代小程序代码提审能力被限制", "服务商资质问题，联系微信"},
	86069:   {ClassPermanent, "owner_setting 必填字段缺失", "补齐隐私指引 owner_setting"},
	86070:   {ClassPermanent, "notice_method 必填字段缺失", "补齐 notice_method"},
	86072:   {ClassPermanent, "store_expire_timestamp 无效", "存储期限需为「数字+天」格式"},
	86073:   {ClassPermanent, "ext_file_media_id 无效", "重新上传隐私指引文件"},
	86074:   {ClassPermanent, "现网隐私协议不存在", "没有现网版时不要传 privacy_ver=1"},
	86075:   {ClassPermanent, "现网隐私协议的 ext_file_media_id 禁止修改", "改开发版后重新发布"},
	86100:   {ClassPermanent, "域名协议头有误", "域名不要带 http:// 等协议前缀"},
	86101:   {ClassPermanent, "不支持配置 api.weixin.qq.com", "更换域名"},
	86102:   {ClassRateLimited, "域名每月修改次数已用尽（50 次）", "下月再改"},
	87006:   {ClassPermanent, "小游戏不能提交代码审核", "小游戏不支持该能力"},
	87012:   {ClassPermanent, "禁止回退该版本", "无上一个线上版本或该版本已回退过"},
	87013:   {ClassRateLimited, "撤回审核次数已达上限（每天 5 次、每月 10 次）", "次日恢复（每天额度 0 点生效）"},
	89014:   {ClassPermanent, "基础库版本输入错误", "传入已发布的基础库版本号"},
	89019:   {ClassPermanent, "业务域名无更改", "无需重复设置"},
	89020:   {ClassPermanent, "尚未设置小程序业务域名", "先在第三方平台登记业务域名"},
	89021:   {ClassPermanent, "域名不是第三方平台已登记的业务域名或其子域名", "先登记同名或父级域名"},
	89029:   {ClassPermanent, "业务域名数量超过限制（最多 300）", "精简业务域名"},
	89231:   {ClassPermanent, "个人小程序不支持设置业务域名", "该主体不支持，跳过此项"},
	89401:   {ClassRetryable, "系统不稳定", "稍后重试"},
	89402:   {ClassPermanent, "该小程序不在待审核队列", "确认已提审且仍在审核中"},
	89403:   {ClassPermanent, "该单不支持加急", "平台不支持此类加急"},
	89404:   {ClassPermanent, "该单已加速成功", "不要重复加急"},
	89405:   {ClassRateLimited, "本月加急额度已用完", "提高提审质量以获取更多额度"},
	9402202: {ClassRateLimited, "请勿频繁提交，待上一次操作完成后再提交", "同一小程序的上传/提审需串行，本平台已按 appid 加锁"},
	9402203: {ClassPermanent, "标准模板 ext_json 错误", "标准模板仅支持 {extAppid, ext, window}；建议改用普通模板"},
	9400001: {ClassPermanent, "该开发小程序已开通直播权限，不支持发布版本", "解绑开发小程序后再操作"},
	45104:   {ClassRateLimited, "第三方平台域名每月修改次数已用尽（50 次）", "下月再改"},
	65316:   {ClassPermanent, "第三方平台域名配置已达上限（业务域名 300）", "删除不用的域名再新增"},
	9410016: {ClassPermanent, "存在无效域名", "按 errmsg 提示修正域名"},
	53300:   {ClassRateLimited, "类目超出每月修改次数限制", "下月再改"},
	53301:   {ClassPermanent, "超出可配置类目总数限制", "删除不用的类目"},
	53302:   {ClassPermanent, "当前账号主体类型不允许设置此种类目", "更换类目"},
	53303:   {ClassPermanent, "提交的参数不合法", "检查类目参数"},
	53304:   {ClassPermanent, "与已有类目重复", "无需重复添加"},
	53305:   {ClassPermanent, "含未通过 ICP 校验的类目", "先完成备案"},
	53306:   {ClassPermanent, "只允许修改类目资质，不允许修改类目 ID", "保持类目 ID 不变"},
	53307:   {ClassPermanent, "只有审核失败的类目允许修改", "等待审核结果"},
	53308:   {ClassPermanent, "审核中的类目不允许删除", "等审核结束后再删除"},
	53310:   {ClassPermanent, "类目数量超上限", "可提交 apply_reason 申请更多"},
	53311:   {ClassPermanent, "需要提交类目资质资料", "补充资质材料"},
	40170:   {ClassPermanent, "拉取数量超出限制", "api_get_authorizer_list 的 count 最大 500"},
	89000:   {ClassPermanent, "该账号已绑定开放平台账号", "无需重复绑定"},
	89001:   {ClassPermanent, "主体不相同", "只能绑定同主体账号"},
	89004:   {ClassPermanent, "开放平台账号绑定数量已达上限（100）", "解绑不用的账号"},
	89036:   {ClassPermanent, "open 账号非接口创建或未认证", "不适用于该场景"},
}

// ErrcodeRule 查询返回码规则。
func ErrcodeRule(code int) (ErrorClass, string, string, bool) {
	r, ok := errcodeRules[code]
	if !ok {
		return ClassUnknown, fmt.Sprintf("未在本地错误码表中登记的微信返回码 %d", code), "请对照官方全局返回码文档处理", false
	}
	return r.Class, r.Text, r.Hint, true
}

// ClassifyErrcode 返回返回码的处置分类；未知码归入 ClassUnknown。
func ClassifyErrcode(code int) ErrorClass {
	class, _, _, _ := ErrcodeRule(code)
	return class
}

// ErrcodeText 返回返回码的中文说明（未知码给出通用文案）。
func ErrcodeText(code int) string {
	_, text, _, _ := ErrcodeRule(code)
	return text
}

// RetryableOnce 报告未知码是否值得再试一次（仅一次，避免放大未知故障）。
func RetryableOnce(code int) bool {
	return ClassifyErrcode(code) == ClassUnknown
}
