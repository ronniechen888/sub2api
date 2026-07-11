# Ubuntu Server 部署文档

本文档适用于以下场景：

- 服务器系统：Ubuntu Server 26.02
- 服务器上已经安装并运行 PostgreSQL 和 Redis
- 服务器上已经克隆好本仓库
- 目标部署方式：源码编译二进制 + systemd 管理服务

> 说明：本项目也支持 Docker Compose 部署，但你当前服务器已经有 PostgreSQL 和 Redis，优先建议使用宿主机已有数据库服务，避免再启动一套容器内数据库。

## 1. 确认系统服务

在服务器上执行：

```bash
sudo systemctl status postgresql
sudo systemctl status redis-server
```

如果服务未运行：

```bash
sudo systemctl enable --now postgresql
sudo systemctl enable --now redis-server
```

确认端口：

```bash
ss -lntp | grep -E '5432|6379'
```

## 2. 准备 PostgreSQL 数据库和用户

进入 PostgreSQL：

```bash
sudo -u postgres psql
```

创建数据库用户和数据库：

```sql
CREATE USER sub2api WITH PASSWORD '请替换为强密码';
CREATE DATABASE sub2api OWNER sub2api;
GRANT ALL PRIVILEGES ON DATABASE sub2api TO sub2api;
\q
```

测试连接：

```bash
psql "host=127.0.0.1 port=5432 user=sub2api password=请替换为强密码 dbname=sub2api sslmode=disable" -c "select 1;"
```

如果 PostgreSQL 只允许 peer 认证或本地 socket 认证，需要检查：

```bash
sudo nano /etc/postgresql/*/main/pg_hba.conf
```

确保存在类似配置：

```conf
host    sub2api    sub2api    127.0.0.1/32    scram-sha-256
```

修改后重载：

```bash
sudo systemctl reload postgresql
```

## 3. 确认 Redis 配置

如果 Redis 没有密码，后续配置里 `redis.password` 留空即可。

测试 Redis：

```bash
redis-cli ping
```

如果 Redis 设置了密码：

```bash
redis-cli -a '你的Redis密码' ping
```

## 4. 安装编译依赖

项目后端是 Go，前端是 Node/pnpm。源码编译时需要安装：

- Go 1.26.3 或兼容版本
- Node.js 24 或兼容版本
- pnpm 9
- git、make、ca-certificates、tzdata

示例：

```bash
sudo apt update
sudo apt install -y git make curl ca-certificates tzdata postgresql-client redis-tools
```

安装 Node.js 后启用 pnpm：

```bash
corepack enable
corepack prepare pnpm@9 --activate
pnpm --version
```

确认 Go 版本：

```bash
go version
```

如果系统仓库没有足够新的 Go/Node，建议使用官方二进制或版本管理工具安装。

## 5. 编译前端

进入仓库根目录：

```bash
cd /path/to/sub2api
```

安装前端依赖并编译：

```bash
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run build
```

编译成功后，应生成：

```bash
ls backend/internal/web/dist
```

## 6. 编译后端二进制

必须使用 `-tags embed`，否则前端页面不会被打进后端二进制。

```bash
cd /path/to/sub2api/backend
VERSION="$(tr -d '\r\n' < ./cmd/server/VERSION)"
CGO_ENABLED=0 go build \
  -tags embed \
  -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildType=release" \
  -trimpath \
  -o bin/sub2api \
  ./cmd/server
```

验证：

```bash
./bin/sub2api -version
```

## 7. 创建运行用户和目录

```bash
sudo useradd --system --home /opt/sub2api --shell /usr/sbin/nologin sub2api || true
sudo mkdir -p /opt/sub2api /etc/sub2api /var/lib/sub2api
sudo cp /path/to/sub2api/backend/bin/sub2api /opt/sub2api/sub2api
sudo chmod +x /opt/sub2api/sub2api
sudo chown -R sub2api:sub2api /opt/sub2api /var/lib/sub2api
sudo chmod 750 /opt/sub2api /var/lib/sub2api
```

## 8. 首次初始化配置

项目支持首次启动时进入 Web 设置向导，也支持环境变量自动初始化。服务器部署建议用自动初始化，避免向导暴露在公网。

生成密钥：

```bash
openssl rand -hex 32
openssl rand -hex 32
```

第一个用作 `JWT_SECRET`，第二个用作 `TOTP_ENCRYPTION_KEY`。

手动执行一次初始化并启动服务进程：

```bash
sudo -u sub2api env \
  DATA_DIR=/var/lib/sub2api \
  AUTO_SETUP=true \
  SERVER_HOST=0.0.0.0 \
  SERVER_PORT=8080 \
  SERVER_MODE=release \
  TZ=Asia/Shanghai \
  DATABASE_HOST=127.0.0.1 \
  DATABASE_PORT=5432 \
  DATABASE_USER=sub2api \
  DATABASE_PASSWORD='请替换为PostgreSQL密码' \
  DATABASE_DBNAME=sub2api \
  DATABASE_SSLMODE=disable \
  REDIS_HOST=127.0.0.1 \
  REDIS_PORT=6379 \
  REDIS_PASSWORD='' \
  REDIS_DB=0 \
  ADMIN_EMAIL='admin@example.com' \
  ADMIN_PASSWORD='请替换为后台管理员密码' \
  JWT_SECRET='请替换为第一个openssl随机值' \
  TOTP_ENCRYPTION_KEY='请替换为第二个openssl随机值' \
  /opt/sub2api/sub2api
```

看到服务启动成功后，按 `Ctrl+C` 停止。此时会在 `/var/lib/sub2api` 生成：

```bash
/var/lib/sub2api/config.yaml
/var/lib/sub2api/.installed
```

检查配置文件权限：

```bash
sudo chown -R sub2api:sub2api /var/lib/sub2api
sudo chmod 600 /var/lib/sub2api/config.yaml
```

## 9. 创建 systemd 服务

创建服务文件：

```bash
sudo nano /etc/systemd/system/sub2api.service
```

写入：

```ini
[Unit]
Description=Sub2API - AI API Gateway Platform
After=network.target postgresql.service redis-server.service
Wants=postgresql.service redis-server.service

[Service]
Type=simple
User=sub2api
Group=sub2api
WorkingDirectory=/opt/sub2api
Environment=DATA_DIR=/var/lib/sub2api
Environment=GIN_MODE=release
Environment=SERVER_HOST=0.0.0.0
Environment=SERVER_PORT=8080
ExecStart=/opt/sub2api/sub2api
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=sub2api

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/opt/sub2api /var/lib/sub2api

[Install]
WantedBy=multi-user.target
```

启用并启动：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now sub2api
```

查看状态：

```bash
sudo systemctl status sub2api
```

查看日志：

```bash
sudo journalctl -u sub2api -f
```

## 10. 验证服务

本机验证：

```bash
curl -i http://127.0.0.1:8080/health
```

浏览器访问：

```text
http://服务器IP:8080
```

如果用了云服务器安全组、防火墙或 UFW，需要放行端口：

```bash
sudo ufw allow 8080/tcp
sudo ufw status
```

## 11. 配置反向代理和 HTTPS

生产环境建议不要直接暴露 `8080`，建议使用 Nginx/Caddy 反向代理到本机：

```text
公网 HTTPS -> Nginx/Caddy -> 127.0.0.1:8080
```

如果使用 Nginx，注意保留 WebSocket 和流式响应相关头：

```nginx
server {
    listen 80;
    server_name your-domain.com;

    client_max_body_size 256m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_buffering off;
        proxy_read_timeout 900s;
    }
}
```

配置 HTTPS 后，建议在 `/var/lib/sub2api/config.yaml` 中设置：

```yaml
server:
  frontend_url: "https://your-domain.com"
```

然后重启：

```bash
sudo systemctl restart sub2api
```

## 12. 日常维护命令

查看状态：

```bash
sudo systemctl status sub2api
```

查看日志：

```bash
sudo journalctl -u sub2api -f
```

重启：

```bash
sudo systemctl restart sub2api
```

停止：

```bash
sudo systemctl stop sub2api
```

查看当前监听端口：

```bash
ss -lntp | grep 8080
```

## 13. 后续快速更新和重新部署

适用于这种情况：你在本地更新代码并已经 `git push origin main`，服务器上需要拉取最新代码、重新编译前后端、替换二进制并重启服务。

### 13.1 手动更新流程

进入仓库：

```bash
cd /path/to/sub2api
git status
git pull --ff-only origin main
```

> 如果 `git pull --ff-only` 提示服务器上有本地改动，先不要强行覆盖。执行 `git status` 看清楚改动来源，确认不需要保留后再处理。

重新编译前端：

```bash
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run build
```

重新编译后端：

```bash
cd /path/to/sub2api/backend
VERSION="$(tr -d '\r\n' < ./cmd/server/VERSION)"
CGO_ENABLED=0 go build \
  -tags embed \
  -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildType=release" \
  -trimpath \
  -o bin/sub2api \
  ./cmd/server
```

替换二进制并重启：

```bash
sudo cp /opt/sub2api/sub2api /opt/sub2api/sub2api.bak.$(date +%Y%m%d%H%M%S)
sudo systemctl stop sub2api
sudo cp /path/to/sub2api/backend/bin/sub2api /opt/sub2api/sub2api
sudo chmod +x /opt/sub2api/sub2api
sudo chown sub2api:sub2api /opt/sub2api/sub2api
sudo systemctl start sub2api
sudo systemctl status sub2api
curl -i http://127.0.0.1:8080/health
```

查看启动日志：

```bash
sudo journalctl -u sub2api -n 100 --no-pager
sudo journalctl -u sub2api -f
```

### 13.2 一键更新脚本

可以在服务器上创建脚本：

```bash
sudo nano /usr/local/bin/deploy-sub2api
```

写入以下内容，把 `APP_REPO` 改成你的服务器仓库实际路径：

```bash
#!/usr/bin/env bash
set -euo pipefail

APP_REPO="/path/to/sub2api"
APP_SERVICE="sub2api"
APP_BIN_SOURCE="${APP_REPO}/backend/bin/sub2api"
APP_BIN_TARGET="/opt/sub2api/sub2api"

cd "${APP_REPO}"

echo "==> Check working tree"
git status --short

if [ -n "$(git status --porcelain)" ]; then
  echo "服务器仓库存在未提交改动，请先处理后再部署。"
  exit 1
fi

echo "==> Pull latest code"
git pull --ff-only origin main

echo "==> Build frontend"
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run build

echo "==> Build backend"
cd "${APP_REPO}/backend"
VERSION="$(tr -d '\r\n' < ./cmd/server/VERSION)"
CGO_ENABLED=0 go build \
  -tags embed \
  -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildType=release" \
  -trimpath \
  -o bin/sub2api \
  ./cmd/server

echo "==> Replace binary"
sudo cp "${APP_BIN_TARGET}" "${APP_BIN_TARGET}.bak.$(date +%Y%m%d%H%M%S)"
sudo systemctl stop "${APP_SERVICE}"
sudo cp "${APP_BIN_SOURCE}" "${APP_BIN_TARGET}"
sudo chmod +x "${APP_BIN_TARGET}"
sudo chown sub2api:sub2api "${APP_BIN_TARGET}"

echo "==> Restart service"
sudo systemctl start "${APP_SERVICE}"
sudo systemctl status "${APP_SERVICE}" --no-pager

echo "==> Health check"
curl -fsS http://127.0.0.1:8080/health
echo
```

赋予执行权限：

```bash
sudo chmod +x /usr/local/bin/deploy-sub2api
```

以后每次本地代码推送后，登录服务器执行：

```bash
deploy-sub2api
```

### 13.3 回滚到上一个二进制

如果更新后服务异常，可以先查看备份文件：

```bash
ls -lh /opt/sub2api/sub2api.bak.*
```

选择最近一次备份恢复：

```bash
sudo systemctl stop sub2api
sudo cp /opt/sub2api/sub2api.bak.替换为具体时间戳 /opt/sub2api/sub2api
sudo chmod +x /opt/sub2api/sub2api
sudo chown sub2api:sub2api /opt/sub2api/sub2api
sudo systemctl start sub2api
sudo systemctl status sub2api
```

## 14. 备份建议

备份数据库：

```bash
pg_dump "host=127.0.0.1 port=5432 user=sub2api password=请替换为PostgreSQL密码 dbname=sub2api sslmode=disable" > sub2api_$(date +%F).sql
```

备份配置：

```bash
sudo tar -czf sub2api_config_$(date +%F).tar.gz /var/lib/sub2api
```

至少需要备份：

- PostgreSQL 数据库
- `/var/lib/sub2api/config.yaml`
- `/var/lib/sub2api/.installed`

## 15. 常见问题

### 页面显示 Frontend not embedded

原因：后端编译时没有加 `-tags embed`，或没有先构建前端。

处理：

```bash
pnpm --dir frontend run build
cd backend
CGO_ENABLED=0 go build -tags embed -o bin/sub2api ./cmd/server
sudo systemctl restart sub2api
```

### 启动后进入 Setup Wizard

原因：`DATA_DIR` 指向的目录里没有 `config.yaml` 或 `.installed`。

检查：

```bash
sudo ls -la /var/lib/sub2api
sudo systemctl cat sub2api
```

确保 systemd 服务里有：

```ini
Environment=DATA_DIR=/var/lib/sub2api
```

### PostgreSQL 连接失败

检查：

```bash
sudo systemctl status postgresql
psql "host=127.0.0.1 port=5432 user=sub2api password=请替换为PostgreSQL密码 dbname=sub2api sslmode=disable" -c "select 1;"
sudo tail -n 100 /var/log/postgresql/postgresql-*.log
```

### Redis 连接失败

检查：

```bash
sudo systemctl status redis-server
redis-cli ping
```

如果 Redis 有密码，确认 `/var/lib/sub2api/config.yaml` 里的 `redis.password` 正确。

### 端口无法访问

检查：

```bash
sudo systemctl status sub2api
ss -lntp | grep 8080
sudo ufw status
```

云服务器还要检查云厂商安全组是否放行对应端口。

## 16. Docker Compose 备选方案

如果后续想改用项目自带 Docker Compose，可以使用 `deploy/` 目录。

项目文档推荐：

```bash
cd /path/to/sub2api/deploy
cp .env.example .env
nano .env
docker compose -f docker-compose.local.yml up -d
```

但注意：默认 Compose 会启动自己的 PostgreSQL 和 Redis，不会复用你宿主机已经安装的服务。除非你手动改 Compose，把 `DATABASE_HOST`、`REDIS_HOST` 指向宿主机服务，并移除内置 `postgres`、`redis` 服务依赖。
