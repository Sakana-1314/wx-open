package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wx-platform/server/internal/auth"
	"wx-platform/server/internal/handler"
)

// TestHealthzAcceptsGetAndHead 锁定健康检查必须同时接受 GET 与 HEAD。
//
// 背景：Docker/K8s/负载均衡的探针普遍用 HEAD（`wget --spider`、`curl -I`）。
// 只注册 GET 时它们会拿到 405，容器永远无法进入 healthy，
// 依赖它的 web 容器也就永远起不来（compose 的 service_healthy 门控会一直等）。
func TestHealthzAcceptsGetAndHead(t *testing.T) {
	// 这里只验证路由注册，容器可以为 nil：健康检查不依赖任何业务依赖。
	am := auth.NewManager("test-secret", time.Hour, "test-password")
	engine := NewRouter(handler.NewServer(nil), am, nil)

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/healthz", nil)
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s /healthz 状态码应为 200，实际 %d（探针会判定为不健康）", method, rec.Code)
			}
			if got := rec.Header().Get("Content-Type"); got == "" {
				t.Fatalf("%s /healthz 缺少 Content-Type", method)
			}
		})
	}
}
