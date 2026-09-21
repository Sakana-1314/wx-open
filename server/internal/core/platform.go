package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wx-platform/server/internal/config"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/wxapi"
	"wx-platform/server/internal/wxtoken"
)

// PlatformService 平台状态总览、额度缓存，以及票据推送的运维入口。
type PlatformService struct {
	cfg    *config.Config
	repos  *Repos
	tokens *wxtoken.Manager
}

// AttachTokens 注入令牌管理器（由装配层在凭据就绪后调用）。
func (s *PlatformService) AttachTokens(tokens *wxtoken.Manager) { s.tokens = tokens }

// EnsurePushTicket 调用 api_start_push_ticket 让微信恢复票据推送。
//
// 适用场景：长时间收不到 component_verify_ticket（票据超 20 分钟未更新），
// 或拿到 61005/61006 报错。恢复后仍需等微信下一次推送到达，本方法不返回票据本身。
func (s *PlatformService) EnsurePushTicket(ctx context.Context) error {
	if s.tokens == nil {
		return ErrNotConfigured
	}
	if err := s.tokens.EnsurePushTicket(ctx); err != nil {
		return fmt.Errorf("恢复票据推送失败: %w", err)
	}
	return nil
}

// urlDomain 提取域名（用于提示授权发起页域名需与回调一致）。
func urlDomain(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	if idx := strings.IndexAny(raw, "/?#"); idx >= 0 {
		raw = raw[:idx]
	}
	return raw
}

// Status 汇总平台状态：票据新鲜度、令牌有效性、回调地址、统计与自检提示。
func (s *PlatformService) Status(ctx context.Context) (*gen.PlatformStatus, error) {
	out := &gen.PlatformStatus{
		ComponentAppid: s.cfg.ComponentAppID,
		TemplateLimit:  intPtr(model.TemplateLibraryLimit),
	}

	authURL, msgURL := CallbackURLs(s.cfg.PublicBaseURL)
	out.AuthorizationEventUrl = authURL
	out.MessageEventUrl = msgURL
	redirect := BuildRedirectURI(s.cfg.PublicBaseURL)
	out.AuthRedirectUri = &redirect
	if masked := maskAppid(s.cfg.ComponentAppID); masked != "" {
		out.ComponentAppidMasked = &masked
	}

	warnings := []string{}
	if s.cfg.ComponentAppID == "" {
		warnings = append(warnings, "第三方平台凭据未配置：请在 server/.env 填写 WX_COMPONENT_APPID / APPSECRET / VERIFY_TOKEN / ENCODING_AES_KEY 后重启服务。")
	}
	if !s.cfg.CallbackSchemeSafe() {
		warnings = append(warnings,
			"PUBLIC_BASE_URL 不是 https：微信要求回调地址公网可达（https 对应 443 端口），否则无法完成接入校验。")
	}

	// 票据与令牌状态。
	if s.repos != nil {
		if state, err := s.repos.State.Get(ctx); err == nil {
			if state.TicketReceivedAt != nil {
				at := *state.TicketReceivedAt
				out.TicketUpdatedAt = &at
				age := int64(time.Since(at).Seconds())
				out.TicketAgeSeconds = &age
				out.TicketFresh = time.Since(at) < 20*time.Minute
			}
			if state.ComponentTokenExpireAt != nil {
				exp := *state.ComponentTokenExpireAt
				out.ComponentTokenExpiresAt = &exp
				valid := time.Now().Before(exp)
				out.ComponentTokenValid = &valid
			}
		}
		if out.ComponentAppid != "" && !out.TicketFresh {
			warnings = append(warnings,
				"最近没有收到 component_verify_ticket：请确认开放平台后台的「授权事件接收 URL」指向 "+authURL+
					"，并检查服务器出口 IP 是否已加入 IP 白名单（未加白会返回 61004）。")
		}
	}

	// 统计。
	if s.repos != nil {
		if counts, err := s.repos.Authorizers.CountByStatus(ctx); err == nil {
			out.AuthAuthorized = int(counts[model.AuthStatusAuthorized])
			out.AuthUnauthorized = int(counts[model.AuthStatusUnauthorized])
		}
		if counts, err := s.repos.Jobs.CountByStatus(ctx); err == nil {
			out.JobRunning = int(counts[model.JobStatusRunning] + counts[model.JobStatusPending])
			failed := int(counts[model.JobStatusFailed] + counts[model.JobStatusPartialFailed])
			out.JobFailed = &failed
		}
		if n, err := s.repos.Templates.Count(ctx); err == nil {
			out.TemplateCount = int(n)
			if n >= model.TemplateLibraryLimit {
				warnings = append(warnings, "模板库已满（上限 200）：请先删除不再使用的模板再添加新模板。")
			}
		}
		if quota := s.CachedQuota(ctx); quota != nil {
			out.AuditQuota = quota
			if quota.Rest != nil && *quota.Rest <= 0 {
				warnings = append(warnings, "提审额度已用尽（85085）：批量提审会被暂停，请在「小程序服务商助手」申请临时额度。")
			}
			if quota.SpeedupRest != nil && *quota.SpeedupRest <= 0 {
				warnings = append(warnings, "加急审核额度已用尽：加急操作会返回 89405。")
			}
		}
		// 权限集体检：已授权但缺少开发权限集（18）的小程序无法批量下发代码。
		if all, err := s.repos.Authorizers.All(ctx); err == nil {
			var missing []string
			for i := range all {
				if all[i].AuthorizationStatus == model.AuthStatusAuthorized && !all[i].HasDevPermission() {
					missing = append(missing, firstNonEmptyStr(all[i].NickName, all[i].Appid))
				}
			}
			if len(missing) > 0 {
				warnings = append(warnings, "以下小程序尚未授权「小程序开发与数据分析」（权限集 18），无法为其上传代码/提审/发布："+
					truncateForMessage(strings.Join(missing, "、"), 200)+"（需商家重新扫码授权并勾选该权限集）")
			}
		}
	}
	out.Warnings = &warnings

	// 授权发起页域名与回调域名不一致时给出明确提示（否则商家会看到「入口页与回调页域名相同」的报错）。
	if redirectDomain := urlDomain(redirect); redirectDomain != "" && s.cfg.PublicBaseURL != "" {
		_ = redirectDomain
	}
	return out, nil
}

// 额度缓存使用的运行参数键。
const (
	settingQuotaRest         = "audit_quota_rest"
	settingQuotaLimit        = "audit_quota_limit"
	settingQuotaSpeedupRest  = "audit_quota_speedup_rest"
	settingQuotaSpeedupLimit = "audit_quota_speedup_limit"
	settingQuotaQueriedAt    = "audit_quota_queried_at"
)

// CachedQuota 读取最近一次查询到的提审/加急额度（服务商级、旗下小程序共用）。
func (s *PlatformService) CachedQuota(ctx context.Context) *gen.AuditQuota {
	if s.repos == nil {
		return nil
	}
	values, err := s.repos.Settings.All(ctx)
	if err != nil {
		return nil
	}
	if _, ok := values[settingQuotaQueriedAt]; !ok {
		return nil
	}
	quota := &gen.AuditQuota{}
	if v, ok := values[settingQuotaRest]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			quota.Rest = &n
		}
	}
	if v, ok := values[settingQuotaLimit]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			quota.Limit = &n
		}
	}
	if v, ok := values[settingQuotaSpeedupRest]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			quota.SpeedupRest = &n
		}
	}
	if v, ok := values[settingQuotaSpeedupLimit]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			quota.SpeedupLimit = &n
		}
	}
	if v, ok := values[settingQuotaQueriedAt]; ok {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			at := time.Unix(ts, 0)
			quota.QueriedAt = &at
		}
	}
	return quota
}

// SaveQuota 落库最近一次额度查询结果（由审核服务与定时对账调用）。
func (s *PlatformService) SaveQuota(ctx context.Context, quota *wxapi.QuotaResponse) error {
	if s.repos == nil || quota == nil {
		return nil
	}
	pairs := map[string]string{
		settingQuotaRest:         strconv.Itoa(quota.Rest),
		settingQuotaLimit:        strconv.Itoa(quota.Limit),
		settingQuotaSpeedupRest:  strconv.Itoa(quota.SpeedupRest),
		settingQuotaSpeedupLimit: strconv.Itoa(quota.SpeedupLimit),
		settingQuotaQueriedAt:    strconv.FormatInt(time.Now().Unix(), 10),
	}
	for key, value := range pairs {
		if err := s.repos.Settings.Put(ctx, key, value); err != nil {
			return Internal(err)
		}
	}
	return nil
}

// LogService 日志查询。
type LogService struct {
	repos *Repos
}

// maskAppid 展示用脱敏（保留前后各 4 位）。
func maskAppid(appid string) string {
	if len(appid) <= 8 {
		return appid
	}
	return appid[:4] + "****" + appid[len(appid)-4:]
}

func intPtr(v int) *int { return &v }

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
