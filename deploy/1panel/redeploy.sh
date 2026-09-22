#!/usr/bin/env bash
# 微信第三方平台管理平台 —— 目标主机（1Panel）上的升级脚本。
#
# 用途：拉取最新镜像并重启编排栈。镜像 tag 固定为 server / web，
# 因此这里只需 pull + up -d，不需要改 compose 文件。
#
# 用法（在目标主机上）：
#   bash /opt/1panel/docker/compose/wx-open/redeploy.sh
#
# 说明：
#   - ghcr.io 在本机不可直连（TLS 被重置），默认走南京大学镜像 ghcr.nju.edu.cn；
#     若该镜像卡在某一层，脚本会依次回退到 ghcr.dockerproxy.net。
#   - .env 含真实密钥（首次部署时生成），不要删、不要提交到仓库。
set -euo pipefail

cd "$(dirname "$0")"

PRIMARY="ghcr.nju.edu.cn/sakana-1314/wx-open"
FALLBACK="ghcr.dockerproxy.net/sakana-1314/wx-open"

pull_component() {
  local component="$1"
  echo "==> 拉取 ${component}"
  if timeout 240 docker pull "${PRIMARY}:${component}"; then
    return 0
  fi
  echo "!! ${PRIMARY}:${component} 拉取失败，回退到 ${FALLBACK}"
  timeout 300 docker pull "${FALLBACK}:${component}"
  # 回退镜像与 nju 的 manifest 完全一致，重新打回 compose 使用的 tag。
  docker tag "${FALLBACK}:${component}" "${PRIMARY}:${component}"
}

pull_component server
pull_component web

docker compose up -d

echo "==> 等待健康检查"
for _ in $(seq 1 30); do
  server="$(docker inspect WxOpen-Server --format '{{.State.Health.Status}}' 2>/dev/null || echo missing)"
  web="$(docker inspect WxOpen-Web --format '{{.State.Health.Status}}' 2>/dev/null || echo missing)"
  printf '  server=%s web=%s\n' "$server" "$web"
  if [ "$server" = healthy ] && [ "$web" = healthy ]; then
    echo "✅ 升级完成：http://$(hostname -I | awk '{print $1}'):24758"
    exit 0
  fi
  sleep 5
done

echo "❌ 超时仍未健康，请查看日志：docker logs WxOpen-Server --tail 50" >&2
exit 1
