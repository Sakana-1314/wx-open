package service

import (
	"errors"

	"wx-platform/server/internal/auth"
	"wx-platform/server/internal/core"
)

// AuthService 平台登录。
type AuthService struct {
	manager *auth.Manager
	limiter *auth.LoginLimiter
}

// Login 校验账号密码并签发 JWT。
//
// 限流在服务层做：无论密码对错都消耗一次配额，避免用响应差异探测口令。
func (s *AuthService) Login(username, password string) (string, error) {
	if !s.limiter.Allow() {
		return "", core.Conflict("登录尝试过于频繁，请稍后再试")
	}
	if !s.manager.Authenticate(username, password) {
		return "", errors.Join(core.ErrUnauthorized, errors.New("用户名或密码错误"))
	}
	token, err := s.manager.Sign(auth.AdminUsername, auth.UserTypeAdmin)
	if err != nil {
		return "", core.Internal(err)
	}
	return token, nil
}

// CurrentUser 返回当前登录用户名。
func (s *AuthService) CurrentUser() string { return auth.AdminUsername }
