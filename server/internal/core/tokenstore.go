package core

import (
	"context"
	"time"

	"wx-platform/server/internal/model"
)

// TokenStore 把数据访问层适配成 wxtoken 需要的接口。
//
// 之所以需要适配：令牌与平台状态在库里分成 platform_state 与 token_cache 两张表，
// 而 wxtoken 只关心「按作用域读写令牌」这一语义。
type TokenStore struct {
	Repos *Repos
}

// NewTokenStore 构造适配器。
func NewTokenStore(repos *Repos) *TokenStore { return &TokenStore{Repos: repos} }

// GetState 读取平台状态（不存在时由 repo 自动创建）。
func (s *TokenStore) GetState(ctx context.Context) (*model.PlatformState, error) {
	return s.Repos.State.Get(ctx)
}

// SaveTicket 持久化最近可用的 component_verify_ticket。
func (s *TokenStore) SaveTicket(ctx context.Context, ticket string, at time.Time) error {
	return s.Repos.State.SaveTicket(ctx, ticket, at)
}

// SaveComponentToken 持久化 component_access_token（重启后可复用，避免撞每日额度）。
func (s *TokenStore) SaveComponentToken(ctx context.Context, token string, expiresAt time.Time) error {
	if err := s.Repos.State.SaveComponentToken(ctx, token, expiresAt); err != nil {
		return err
	}
	// 同时在 token_cache 留一份，便于状态页统一查询。
	return s.Repos.Tokens.Put(ctx, model.TokenScopeComponent, "", token, expiresAt)
}

// MarkPushOK 记录一次成功的推送时间。
func (s *TokenStore) MarkPushOK(ctx context.Context, at time.Time) error {
	return s.Repos.State.MarkPushOK(ctx, at)
}

// GetToken 读取令牌缓存。
func (s *TokenStore) GetToken(ctx context.Context, scope model.TokenScope, appid string) (*model.TokenCache, error) {
	return s.Repos.Tokens.Get(ctx, scope, appid)
}

// PutToken 写入令牌缓存。
func (s *TokenStore) PutToken(ctx context.Context, scope model.TokenScope, appid, token string, expiresAt time.Time) error {
	return s.Repos.Tokens.Put(ctx, scope, appid, token, expiresAt)
}

// DeleteToken 删除令牌缓存。
func (s *TokenStore) DeleteToken(ctx context.Context, scope model.TokenScope, appid string) error {
	return s.Repos.Tokens.Delete(ctx, scope, appid)
}

// ---- wxtoken.AuthorizerStore ----

// Get 读取授权方。
func (s *TokenStore) Get(ctx context.Context, appid string) (*model.Authorizer, error) {
	return s.Repos.Authorizers.Get(ctx, appid)
}

// UpdateRefreshToken 原子更新 refresh_token 密文。
func (s *TokenStore) UpdateRefreshToken(ctx context.Context, appid string, cipher []byte, at time.Time) error {
	return s.Repos.Authorizers.UpdateRefreshToken(ctx, appid, cipher, at)
}

// ListAll 列出全部授权方（含已取消授权，用于全量重拉令牌时补记录）。
func (s *TokenStore) ListAll(ctx context.Context) ([]model.Authorizer, error) {
	return s.Repos.Authorizers.All(ctx)
}

// Upsert 写入或更新授权方。
func (s *TokenStore) Upsert(ctx context.Context, a *model.Authorizer) error {
	return s.Repos.Authorizers.Upsert(ctx, a)
}
