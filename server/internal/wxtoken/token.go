// Package wxtoken 管理第三方平台的票据与两类令牌，是「不把微信额度撞满」的关键。
//
// 依据官方文档实现的策略：
//   - component_verify_ticket：每 10 分钟推送一次、有效期 12 小时，必须持久化「最近可用」的票据；
//     令牌过期时若拿不到新票据，允许用最近可用的票据兜底（官方明确建议）。
//   - component_access_token：有效期 2 小时，官方建议在 1 小时 50 分（即提前 10 分钟）刷新；
//     必须缓存，否则会撞每日获取限制。
//   - authorizer_access_token：有效期 2 小时，必须缓存；用 authorizer_refresh_token 刷新，
//     响应若带回新的 refresh_token 则原子覆盖（为空则保留旧值）。
//   - 并发单飞：同一 Key 的并发刷新只打一次微信接口，避免多请求互相覆盖令牌。
//   - refresh_token 丢失/失效时，提供 api_get_authorizer_list 全量重拉的官方恢复路径。
package wxtoken

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"wx-platform/server/internal/model"
	"wx-platform/server/internal/secretbox"
	"wx-platform/server/internal/wxapi"
)

// refreshAhead 提前刷新窗口（官方建议提前 10 分钟）。
const refreshAhead = 10 * time.Minute

// Store 令牌与平台状态的持久化接口（由 repo 实现）。
type Store interface {
	GetState(ctx context.Context) (*model.PlatformState, error)
	SaveTicket(ctx context.Context, ticket string, at time.Time) error
	SaveComponentToken(ctx context.Context, token string, expiresAt time.Time) error
	MarkPushOK(ctx context.Context, at time.Time) error
	GetToken(ctx context.Context, scope model.TokenScope, appid string) (*model.TokenCache, error)
	PutToken(ctx context.Context, scope model.TokenScope, appid, token string, expiresAt time.Time) error
	DeleteToken(ctx context.Context, scope model.TokenScope, appid string) error
}

// AuthorizerStore 授权方持久化接口（refresh_token 以密文列存储）。
type AuthorizerStore interface {
	Get(ctx context.Context, appid string) (*model.Authorizer, error)
	UpdateRefreshToken(ctx context.Context, appid string, cipher []byte, at time.Time) error
	ListAll(ctx context.Context) ([]model.Authorizer, error)
	Upsert(ctx context.Context, a *model.Authorizer) error
}

// Status 令牌健康度快照，供概览页与体检使用。
type Status struct {
	ComponentAppid      string     `json:"componentAppid"`
	TicketUpdatedAt     *time.Time `json:"ticketUpdatedAt"`
	TicketAgeSeconds    int64      `json:"ticketAgeSeconds"`
	TicketFresh         bool       `json:"ticketFresh"`
	TokenExpiresAt      *time.Time `json:"tokenExpiresAt"`
	TokenValid          bool       `json:"tokenValid"`
	HasTicket           bool       `json:"hasTicket"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	LastError           string     `json:"lastError"`
}

// ticketFreshWindow 票据新鲜度阈值：超过即认为推送可能已中断（官方推送周期为 10 分钟）。
const ticketFreshWindow = 20 * time.Minute

// Manager 令牌管理器。
type Manager struct {
	api             *wxapi.Client
	store           Store
	authStore       AuthorizerStore
	box             *secretbox.Box
	componentAppid  string
	componentSecret string

	mu           sync.Mutex
	componentTok *cachedToken
	recentTicket string
	ticketAt     time.Time
	lastErr      string
	failures     int

	// inflight 实现按 Key 的单飞刷新。
	inflight map[string]*call
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

// call 单飞调用。
type call struct {
	wg  sync.WaitGroup
	err error
}

// New 构造管理器；componentAppid/Secret 为空表示未配置（此时任何取令牌的操作都会返回 ErrNotConfigured）。
func New(api *wxapi.Client, store Store, authStore AuthorizerStore, box *secretbox.Box, componentAppid, componentSecret string) *Manager {
	return &Manager{
		api:             api,
		store:           store,
		authStore:       authStore,
		box:             box,
		componentAppid:  componentAppid,
		componentSecret: componentSecret,
		inflight:        map[string]*call{},
	}
}

// ErrNotConfigured 凭据未配置。
var ErrNotConfigured = errors.New("第三方平台凭据未配置（WX_COMPONENT_APPID / WX_COMPONENT_APPSECRET 等）")

// ErrNoTicket 尚未收到 component_verify_ticket。
var ErrNoTicket = errors.New("尚未收到 component_verify_ticket：请先在开放平台后台把「授权事件接收 URL」配好并等待微信推送（每 10 分钟一次），或调用 api_start_push_ticket 恢复推送")

// singleflight 按 key 合并并发调用。
func (m *Manager) singleflight(key string, fn func() error) error {
	m.mu.Lock()
	if m.inflight == nil {
		m.inflight = map[string]*call{}
	}
	if c, ok := m.inflight[key]; ok {
		m.mu.Unlock()
		c.wg.Wait()
		return c.err
	}
	c := &call{}
	c.wg.Add(1)
	m.inflight[key] = c
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.inflight, key)
		m.mu.Unlock()
		c.wg.Done()
	}()

	c.err = fn()
	return c.err
}

// Load 启动时把持久化的票据与令牌读入内存，避免重启后立刻再打一次微信接口。
func (m *Manager) Load(ctx context.Context) error {
	state, err := m.store.GetState(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recentTicket = state.VerifyTicket
	if state.TicketReceivedAt != nil {
		m.ticketAt = *state.TicketReceivedAt
	}
	if state.ComponentAccessToken != "" && state.ComponentTokenExpireAt != nil {
		m.componentTok = &cachedToken{token: state.ComponentAccessToken, expiresAt: *state.ComponentTokenExpireAt}
	}
	return nil
}

// SaveTicket 保存回调收到的票据（授权事件接收 URL 每 10 分钟推送一次）。
func (m *Manager) SaveTicket(ctx context.Context, ticket string, at time.Time) error {
	if ticket == "" {
		return fmt.Errorf("票据为空")
	}
	if err := m.store.SaveTicket(ctx, ticket, at); err != nil {
		return err
	}
	m.mu.Lock()
	m.recentTicket = ticket
	m.ticketAt = at
	m.mu.Unlock()
	// 票据更新后，旧令牌应立即作废，强制下次调用用新票据换取。
	m.mu.Lock()
	m.componentTok = nil
	m.mu.Unlock()
	return nil
}

// MarkPushOK 记录一次成功的推送（用于概览页新鲜度与告警）。
func (m *Manager) MarkPushOK(ctx context.Context, at time.Time) error {
	return m.store.MarkPushOK(ctx, at)
}

// RecentTicket 返回最近可用的票据（可能为空）。
func (m *Manager) RecentTicket() (string, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recentTicket, m.ticketAt
}

// EnsurePushTicket 调用 api_start_push_ticket 恢复票据推送（票据长时间收不到时的官方处置手段）。
func (m *Manager) EnsurePushTicket(ctx context.Context) error {
	if m.componentAppid == "" || m.componentSecret == "" {
		return ErrNotConfigured
	}
	return m.api.StartPushTicket(ctx, wxapi.StartPushTicketRequest{
		ComponentAppid:  m.componentAppid,
		ComponentSecret: m.componentSecret,
	})
}

// ComponentToken 获取第三方平台自身令牌（模板库等接口使用）。
func (m *Manager) ComponentToken(ctx context.Context) (string, error) {
	if m.componentAppid == "" || m.componentSecret == "" {
		return "", ErrNotConfigured
	}
	if tok := m.cachedComponentToken(); tok != "" {
		return tok, nil
	}
	var token string
	err := m.singleflight("component_access_token", func() error {
		// 双检：等锁期间可能已被其他协程刷新。
		if tok := m.cachedComponentToken(); tok != "" {
			token = tok
			return nil
		}
		ticket, _ := m.RecentTicket()
		if ticket == "" {
			// 兜底：从数据库再读一次（多实例或重启场景）。
			if state, err := m.store.GetState(ctx); err == nil && state.VerifyTicket != "" {
				ticket = state.VerifyTicket
			}
		}
		if ticket == "" {
			return ErrNoTicket
		}
		resp, err := m.api.ComponentToken(ctx, wxapi.ComponentTokenRequest{
			ComponentAppid:        m.componentAppid,
			ComponentAppsecret:    m.componentSecret,
			ComponentVerifyTicket: ticket,
		})
		if err != nil {
			m.recordFailure(err)
			return err
		}
		expiresAt := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
		if resp.ExpiresIn <= 0 {
			expiresAt = time.Now().Add(2 * time.Hour)
		}
		if err := m.store.SaveComponentToken(ctx, resp.ComponentAccessToken, expiresAt); err != nil {
			return err
		}
		m.mu.Lock()
		m.componentTok = &cachedToken{token: resp.ComponentAccessToken, expiresAt: expiresAt}
		m.failures = 0
		m.lastErr = ""
		m.mu.Unlock()
		token = resp.ComponentAccessToken
		return nil
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// cachedComponentToken 返回仍然有效的缓存令牌（含提前刷新窗口）。
func (m *Manager) cachedComponentToken() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.componentTok == nil {
		return ""
	}
	if time.Now().Add(refreshAhead).Before(m.componentTok.expiresAt) {
		return m.componentTok.token
	}
	return ""
}

// AuthorizerToken 获取授权方令牌（代码管理接口使用）。
func (m *Manager) AuthorizerToken(ctx context.Context, appid string) (string, error) {
	if cached, ok := m.cachedAuthorizerToken(ctx, appid); ok {
		return cached, nil
	}
	var token string
	err := m.singleflight("authorizer_access_token:"+appid, func() error {
		if cached, ok := m.cachedAuthorizerToken(ctx, appid); ok {
			token = cached
			return nil
		}
		componentToken, err := m.ComponentToken(ctx)
		if err != nil {
			return err
		}
		authorizer, err := m.authStore.Get(ctx, appid)
		if err != nil {
			return err
		}
		refreshToken, err := m.box.Open([]byte(authorizer.RefreshTokenCipher))
		if err != nil {
			return fmt.Errorf("解密 refresh_token 失败: %w", err)
		}
		if refreshToken == "" {
			return fmt.Errorf("小程序 %s 没有可用的 refresh_token，请重新授权或执行「重新拉取令牌」", appid)
		}
		resp, err := m.api.AuthorizerToken(ctx, componentToken, m.componentAppid, appid, refreshToken)
		if err != nil {
			return err
		}
		if resp.AuthorizerRefreshToken != "" && resp.AuthorizerRefreshToken != refreshToken {
			// 官方未承诺刷新后 refresh_token 一定变化；变了就原子覆盖，没变保留旧值。
			cipher, err := m.box.Seal(resp.AuthorizerRefreshToken)
			if err != nil {
				return err
			}
			if err := m.authStore.UpdateRefreshToken(ctx, appid, cipher, time.Now()); err != nil {
				return err
			}
		}
		expiresAt := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
		if resp.ExpiresIn <= 0 {
			expiresAt = time.Now().Add(2 * time.Hour)
		}
		if err := m.store.PutToken(ctx, model.TokenScopeAuthorizer, appid, resp.AuthorizerAccessToken, expiresAt); err != nil {
			return err
		}
		token = resp.AuthorizerAccessToken
		return nil
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// cachedAuthorizerToken 读内存/数据库中的有效授权方令牌。
func (m *Manager) cachedAuthorizerToken(ctx context.Context, appid string) (string, bool) {
	cached, err := m.store.GetToken(ctx, model.TokenScopeAuthorizer, appid)
	if err != nil || cached == nil {
		return "", false
	}
	if time.Now().Add(refreshAhead).Before(cached.ExpiresAt) {
		return cached.Token, true
	}
	return "", false
}

// InvalidateAuthorizer 让某个小程序的令牌立即失效（取消授权、令牌报错时调用）。
func (m *Manager) InvalidateAuthorizer(ctx context.Context, appid string) error {
	return m.store.DeleteToken(ctx, model.TokenScopeAuthorizer, appid)
}

// StoreAuthorizerToken 在换取授权码时直接写入令牌（省一次刷新调用）。
func (m *Manager) StoreAuthorizerToken(ctx context.Context, appid, token string, expiresIn int) error {
	if token == "" {
		return nil
	}
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
	if expiresIn <= 0 {
		expiresAt = time.Now().Add(2 * time.Hour)
	}
	return m.store.PutToken(ctx, model.TokenScopeAuthorizer, appid, token, expiresAt)
}

// ResyncResult 全量重拉 refresh_token 的结果。
type ResyncResult struct {
	Total   int      `json:"total"`
	Updated int      `json:"updated"`
	Missing []string `json:"missing"`
}

// ResyncRefreshTokens 用 api_get_authorizer_list 全量重拉 refresh_token（官方恢复路径）。
func (m *Manager) ResyncRefreshTokens(ctx context.Context) (*ResyncResult, error) {
	componentToken, err := m.ComponentToken(ctx)
	if err != nil {
		return nil, err
	}
	result := &ResyncResult{}
	const pageSize = 500
	for offset := 0; ; offset += pageSize {
		resp, err := m.api.GetAuthorizerList(ctx, componentToken, m.componentAppid, offset, pageSize)
		if err != nil {
			return nil, err
		}
		for _, item := range resp.List {
			result.Total++
			if item.AuthorizerAppid == "" || item.RefreshToken == "" {
				result.Missing = append(result.Missing, item.AuthorizerAppid)
				continue
			}
			cipher, err := m.box.Seal(item.RefreshToken)
			if err != nil {
				return nil, err
			}
			authorizer, err := m.authStore.Get(ctx, item.AuthorizerAppid)
			if err != nil || authorizer == nil {
				// 平台还不知道这个授权方：至少把 refresh_token 记下来，等信息同步补齐资料。
				if err := m.authStore.Upsert(ctx, &model.Authorizer{
					Appid:                 item.AuthorizerAppid,
					RefreshTokenCipher:    string(cipher),
					RefreshTokenUpdatedAt: timePtr(time.Now()),
					AuthorizationStatus:   model.AuthStatusAuthorized,
					Enabled:               true,
					CodeSource:            model.CodeSourceTemplate,
				}); err != nil {
					return nil, err
				}
				result.Updated++
				continue
			}
			if err := m.authStore.UpdateRefreshToken(ctx, item.AuthorizerAppid, cipher, time.Now()); err != nil {
				return nil, err
			}
			result.Updated++
		}
		if len(resp.List) == 0 || offset+pageSize >= resp.TotalCount {
			break
		}
	}
	return result, nil
}

// Status 返回令牌健康度。
func (m *Manager) Status(ctx context.Context) Status {
	m.mu.Lock()
	ticketAt := m.ticketAt
	ticket := m.recentTicket
	componentTok := m.componentTok
	failures := m.failures
	lastErr := m.lastErr
	m.mu.Unlock()

	st := Status{
		ComponentAppid:      m.componentAppid,
		HasTicket:           ticket != "",
		ConsecutiveFailures: failures,
		LastError:           lastErr,
	}
	if !ticketAt.IsZero() {
		at := ticketAt
		st.TicketUpdatedAt = &at
		st.TicketAgeSeconds = int64(time.Since(ticketAt).Seconds())
		st.TicketFresh = time.Since(ticketAt) < ticketFreshWindow
	}
	// 内存里没有就查库（重启后 Load 已填，但令牌可能被其他路径写入）。
	if componentTok == nil {
		if cached, err := m.store.GetToken(ctx, model.TokenScopeComponent, ""); err == nil && cached != nil {
			exp := cached.ExpiresAt
			st.TokenExpiresAt = &exp
			st.TokenValid = time.Now().Before(cached.ExpiresAt)
			return st
		}
		return st
	}
	exp := componentTok.expiresAt
	st.TokenExpiresAt = &exp
	st.TokenValid = time.Now().Before(componentTok.expiresAt)
	return st
}

func (m *Manager) recordFailure(err error) {
	m.mu.Lock()
	m.failures++
	m.lastErr = err.Error()
	m.mu.Unlock()
}

func timePtr(t time.Time) *time.Time { return &t }
