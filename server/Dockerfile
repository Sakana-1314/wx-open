# 后端镜像：构建上下文为 server/ 目录本身（CI 里 context=./server），因此 COPY 路径不含 server/ 前缀。
# Go 版本固定跟随 server/go.mod，升级 go.mod 时同步改这里。
FROM golang:1.27-alpine AS build

WORKDIR /src

# 先只拷贝依赖清单，利用层缓存：源码改动不会重新下载模块。
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG BUILD_TIME
ARG GIT_SHA
# CGO_ENABLED=0 生成静态二进制，才能跑在无 libc 的 alpine 上；-trimpath 去掉本机绝对路径。
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags "-s -w -X main.buildTime=${BUILD_TIME} -X main.gitSHA=${GIT_SHA}" \
      -o /out/wx-platform ./cmd/server

FROM alpine:3

# 时区与根证书：微信 API 走 https，对账与日志按本地时区展示。
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -H -u 10001 wxplatform \
 && mkdir -p /app \
 && chown -R wxplatform:wxplatform /app

WORKDIR /app
COPY --from=build /out/wx-platform /app/wx-platform

# .env 由程序用 godotenv 从工作目录读取；生产建议用环境变量直接注入。
USER wxplatform
EXPOSE 8091

# /healthz 在 router 中注册，不鉴权也不连微信。
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=5 \
  CMD wget -q --spider http://127.0.0.1:8091/healthz || exit 1

ENTRYPOINT ["/app/wx-platform"]
