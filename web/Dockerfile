# 前端镜像：构建上下文为 web/ 目录本身（CI 里 context=./web），因此 COPY 路径不含 web/ 前缀。
# ---- 构建阶段：pnpm + Vite，产出 web/dist ----
FROM node:24-alpine AS build

WORKDIR /app

# 只用 lockfile 装依赖，利用层缓存。
COPY package.json pnpm-lock.yaml ./
RUN corepack enable && corepack prepare pnpm@11 --activate \
 && pnpm install --frozen-lockfile

COPY . .

# HOST 为后端接口地址，构建期经 vite define 注入为 __API_HOST__：
#   留空（默认）→ 前端走相对 /api/v1，由本镜像内的 nginx 反代到 server 容器；
#   填 https://api.example.com → 前端直连该地址，跨域由后端 CORS 处理。
ARG HOST=
ENV HOST=${HOST}
RUN pnpm build

# ---- 运行阶段：nginx 托管静态产物并反代 /api 与 /callback ----
FROM nginx:stable-alpine

# 容器内后端地址：默认按 docker compose 的服务名解析，独立部署时用 --add-host 或环境变量覆盖。
ENV BACKEND_HOST=server
ENV BACKEND_PORT=8091

COPY nginx.conf /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=5 \
  CMD wget -q --spider http://127.0.0.1/ || exit 1
