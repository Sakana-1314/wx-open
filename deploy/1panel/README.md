# 1Panel 主机部署说明（wx-open）

记录 **172.17.1.1 那台 1Panel 主机**上的实际部署方式，便于复现与迁移。
通用部署方式仍见上一级目录的 `docker-compose.yml` 与 README「4.5 容器化部署」。

## 实际部署形态

| 项 | 值 |
|---|---|
| 部署目录 | `/opt/1panel/docker/compose/wx-open/` |
| 容器 | `WxOpen-Server`（内部 `:8091`）、`WxOpen-Web`（宿主 `:24758` → 容器 `:80`） |
| 访问地址 | 局域网 `http://172.17.1.1:24758`（`docker compose` 所在网络内也可用容器名 `web`） |
| 网络 | 外部网络 `1panel-network`（与该主机其它服务同网络，可用容器名互访） |
| 数据库 | 复用主机上已有的 MySQL 容器（容器名 `MySQL`，8.4），库 `wx_platform`、用户 `wxplatform` |
| 镜像 | `ghcr.nju.edu.cn/sakana-1314/wx-open:{server,web}`（该主机直连 ghcr.io 会 TLS 重置） |
| 配置 | 同目录 `.env`（权限 `600`，含真实密钥，**只在主机上**，不入库） |
| 升级 | `bash /opt/1panel/docker/compose/wx-open/redeploy.sh` |

选 24758 的理由：该主机惯用 24xxx 段承接对外服务（SillyTavern 24753、TrackManager 24756、
Chaoxing-EdgeAPIs 24757），24758 与其相邻且已确认空闲。

## 首次部署步骤

```bash
# 1. 主机上建库建用户（密码用 openssl rand -hex 16 生成）
docker exec -i MySQL mysql -uroot -p<root密码> <<'SQL'
CREATE DATABASE IF NOT EXISTS wx_platform CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'wxplatform'@'%' IDENTIFIED BY '<DB_PASSWORD>';
GRANT ALL PRIVILEGES ON wx_platform.* TO 'wxplatform'@'%';
FLUSH PRIVILEGES;
SQL

# 2. 放编排文件与 .env（本目录的 docker-compose.yml 直接拷过去）
mkdir -p /opt/1panel/docker/compose/wx-open
# 拷 docker-compose.yml 与 .env，然后：
chmod 600 /opt/1panel/docker/compose/wx-open/.env

# 3. 起服务
cd /opt/1panel/docker/compose/wx-open && docker compose up -d
```

## 镜像拉取

该主机直连 `ghcr.io` 会在握手阶段被重置（`GnuTLS/read: connection reset by peer`），必须走镜像站：

| 镜像站 | 结果 |
|---|---|
| `ghcr.nju.edu.cn` | ✅ 可用（主机上 SillyTavern 也用它）；偶尔会卡在最后一层，重试即可 |
| `ghcr.dockerproxy.net` | ✅ 可用，作为回退（digest 与 ghcr 一致） |
| `ghcr.m.daocloud.io` | ❌ 白名单限制，本镜像被拒 |

`redeploy.sh` 已内置「先 nju，失败回退 dockerproxy 并重新打 tag」的逻辑。

## 尚未完成：接真实微信

当前 `PUBLIC_BASE_URL` 是局域网地址，`WX_COMPONENT_*` 全为空，因此：

- 概览页会提示「第三方平台凭据未配置」与「PUBLIC_BASE_URL 不是 https」——这是**预期**的；
- 登录、页面、回调路由（无签名返回 400）都可正常工作，但真实微信调用会被拦下。

要真正接入微信，需要：

1. 给该主机配一个 **https 域名**并反代到 `24758`（微信回调只认 80/443 且推荐 https）；
   `PUBLIC_BASE_URL` 改成 `https://<域名>`，与开放平台后台填写的值一字不差；
2. 在 `.env` 填入 `WX_COMPONENT_APPID / APPSECRET / VERIFY_TOKEN / ENCODING_AES_KEY`；
3. 把该主机**公网出口 IP** 加入微信 IP 白名单（否则 `61004`）；
4. 重启：`docker compose up -d --force-recreate server`（环境变量变更不会自动生效）。

回调地址填：
- 授权事件接收 URL：`https://<域名>/callback/component`
- 消息与事件接收 URL：`https://<域名>/callback/message/$APPID$`
