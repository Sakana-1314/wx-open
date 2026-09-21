SHELL := /bin/bash

# Go 未加入 PATH（/usr/local/go/bin 已安装），这里显式注入，保证 make 在任意 shell 下可用。
GO_BIN ?= /usr/local/go/bin
export PATH := $(GO_BIN):$(PATH):$(HOME)/go/bin

SERVER_DIR := server
WEB_DIR := web
API := api/openapi.yaml
OAPI_VERSION := v2.8.0

.PHONY: help tools gen gen-go gen-web db-init dev-backend dev-web build build-backend \
        test test-backend test-web lint check dev e2e-mock clean

help:
	@echo "微信第三方平台管理平台 — 常用命令"
	@echo "  make tools         安装 oapi-codegen（$(OAPI_VERSION)，仅首次需要，需要网络）"
	@echo "  make gen           由 $(API) 重新生成 Go 与 TS 两端代码"
	@echo "  make db-init       建库建用户（读取 db/init.sql，需本机 MySQL root 权限）"
	@echo "  make dev-backend   启动后端（:8091，需先配好 server/.env）"
	@echo "  make dev-web       启动前端开发服务器（:5174）"
	@echo "  make dev           并行启动后端与前端（Ctrl-C 一并退出）"
	@echo "  make build         构建后端二进制与前端产物（bin/wx-platform + web/dist）"
	@echo "  make test          后端 go test + 前端 vitest"
	@echo "  make check         后端 go build/vet/gofmt + 前端 typecheck/build（提交前必跑）"
	@echo "  make e2e-mock      跑 mock 微信端到端集成测试（无需真实凭据）"
	@echo "  make clean         清理构建产物"

tools:
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_VERSION)
	@echo "✅ oapi-codegen 已安装到 $$(go env GOPATH)/bin"

gen: gen-go gen-web

gen-go:
	cd $(SERVER_DIR) && oapi-codegen -generate types,gin -package gen -o internal/gen/api.gen.go ../$(API)
	@echo "✅ 已生成 server/internal/gen/api.gen.go"

gen-web:
	cd $(WEB_DIR) && pnpm schema
	@echo "✅ 已生成 web/src/api/schema.d.ts"

db-init:
	mysql -u root < db/init.sql
	@echo "✅ 已建库 wx_platform / wx_platform_test 与用户 wxplatform"

dev-backend:
	cd $(SERVER_DIR) && go run ./cmd/server

dev-web:
	cd $(WEB_DIR) && pnpm dev

dev:
	@echo "后端 :8091 / 前端 :5174，Ctrl-C 退出"
	@trap 'kill 0' INT TERM; \
	( cd $(SERVER_DIR) && go run ./cmd/server ) & \
	( cd $(WEB_DIR) && pnpm dev ) & \
	wait

build: build-backend

build-backend:
	cd $(SERVER_DIR) && go build -o ../bin/wx-platform ./cmd/server
	@echo "✅ 已构建 bin/wx-platform"

test: test-backend test-web

# -p 1：各集成测试包共用同一个 TEST_DB_DSN 数据库且会清表，串行执行避免互相破坏。
test-backend:
	cd $(SERVER_DIR) && go test -p 1 ./...

test-web:
	cd $(WEB_DIR) && pnpm test

lint:
	cd $(SERVER_DIR) && go vet ./... && gofmt -l ./internal ./cmd

check:
	@cd $(SERVER_DIR) && go build ./... && go vet ./... && \
	  test -z "$$(gofmt -l ./internal ./cmd)" || (echo "❌ gofmt 未通过，请执行 gofmt -w ./internal ./cmd" && exit 1)
	@echo "✅ 后端检查通过"
	cd $(WEB_DIR) && pnpm typecheck && pnpm build
	@echo "✅ 前端检查通过"

# 端到端：内置模拟微信服务端驱动全链路（授权 → 模板库 → 批量上传代码 → 批量提审 → 审核结果推送 → 批量发布）。
# 使用独立库 wx_platform_e2e，避免与各包的集成测试互相干扰（先 make db-init 建库）。
e2e-mock:
	cd $(SERVER_DIR) && MOCK_WX=1 \
	  TEST_DB_DSN='wxplatform:wxplatform_dev_password@tcp(127.0.0.1:3306)/wx_platform_e2e?charset=utf8mb4&parseTime=True&loc=Local' \
	  go test ./internal/e2e/ -v -count=1

clean:
	rm -rf bin $(WEB_DIR)/dist $(WEB_DIR)/node_modules/.tmp $(WEB_DIR)/tsconfig.tsbuildinfo
	@echo "✅ 已清理构建产物"
