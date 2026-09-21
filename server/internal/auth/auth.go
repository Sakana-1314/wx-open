// Package auth 负责 JWT 签发/校验、Gin 鉴权中间件与登录限流。
//
// 用户体系：平台自身只有一个管理用户 admin，密码来自环境变量 ADMIN_PASSWORD（不入库）；
// 微信侧的授权小程序不是平台用户，因此没有多用户/RBAC（列为扩展点）。
package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// UserType 平台用户类型。
type UserType string

// UserTypeAdmin 管理用户。
const UserTypeAdmin UserType = "admin"

// AdminUsername 管理用户名固定为 admin。
const AdminUsername = "admin"

// Claims JWT 载荷。
type Claims struct {
	UserType UserType `json:"user_type"`
	jwt.RegisteredClaims
}

// Manager 签发与校验 JWT。
type Manager struct {
	secret        []byte
	ttl           time.Duration
	adminPassword string
}

// NewManager 构造 Manager。
func NewManager(secret string, ttl time.Duration, adminPassword string) *Manager {
	return &Manager{secret: []byte(secret), ttl: ttl, adminPassword: adminPassword}
}

// Sign 签发 HS256 JWT。
func (m *Manager) Sign(sub string, userType UserType) (string, error) {
	now := time.Now()
	claims := Claims{
		UserType: userType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
			Issuer:    "wx-platform",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("签发 JWT 失败: %w", err)
	}
	return signed, nil
}

// Verify 校验签名与有效期。
func (m *Manager) Verify(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("非预期的签名算法 %v", t.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("令牌校验失败: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("令牌无效")
	}
	return claims, nil
}

// Authenticate 校验用户名密码（恒定时间比较，空密码显式拒绝）。
func (m *Manager) Authenticate(username, password string) bool {
	if username != AdminUsername || password == "" || m.adminPassword == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(m.adminPassword)) == 1
}

// ClaimsFrom 从上下文读取当前用户载荷。
func ClaimsFrom(c *gin.Context) *Claims {
	v, ok := c.Get(ctxClaimsKey)
	if !ok {
		return nil
	}
	claims, ok := v.(*Claims)
	if !ok {
		return nil
	}
	return claims
}

const ctxClaimsKey = "auth.claims"

// JWTMiddleware 校验 Authorization: Bearer 令牌。
func JWTMiddleware(m *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			abortUnauthorized(c, "缺少 Authorization: Bearer 令牌")
			return
		}
		claims, err := m.Verify(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			abortUnauthorized(c, "令牌无效或已过期，请重新登录")
			return
		}
		c.Set(ctxClaimsKey, claims)
		c.Next()
	}
}

// RequireAdmin 要求当前用户是管理用户。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := ClaimsFrom(c)
		if claims == nil || claims.UserType != UserTypeAdmin {
			abortUnauthorized(c, "需要管理用户权限")
			return
		}
		c.Next()
	}
}

func abortUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "message": message})
}

// LoginLimiter 基于内存的滑动窗口登录限流（防止公网上的暴力破解）。
type LoginLimiter struct {
	mu     sync.Mutex
	hits   []time.Time
	window time.Duration
	max    int
}

// NewLoginLimiter 构造限流器；max<=0 表示不限制。
func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	if window <= 0 {
		window = 5 * time.Minute
	}
	return &LoginLimiter{window: window, max: max}
}

// Allow 记录一次尝试并返回是否放行。
func (l *LoginLimiter) Allow() bool {
	if l.max <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	kept := l.hits[:0]
	for _, t := range l.hits {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	l.hits = kept
	if len(l.hits) >= l.max {
		return false
	}
	l.hits = append(l.hits, now)
	return true
}
