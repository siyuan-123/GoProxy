# ProxyGo

一个基于 Go 的轻量代理池服务。程序会从公开代理源抓取 HTTP/SOCKS5 代理，验证可用性后写入 SQLite，并对外暴露一个统一的本地 HTTP 代理入口，同时提供带登录的 Web 管理后台。

## 功能概览

- 启动时自动抓取并验证代理
- 后台定时抓取新代理
- 后台定时健康检查，自动清理不可用代理
- 聚合 HTTP 和 SOCKS5 上游代理，对外统一提供 HTTP 代理端口
- 支持普通 HTTP 请求和 HTTPS `CONNECT` 隧道转发
- 内置 WebUI，支持查看统计、筛选代理、删除代理、手动触发抓取、查看日志、修改部分运行参数
- 使用 SQLite 持久化代理池数据

## 项目结构

```text
.
├── main.go               # 程序入口
├── config/               # 默认配置、配置加载与保存
├── fetcher/              # 代理源抓取
├── validator/            # 代理可用性验证
├── checker/              # 周期健康检查
├── storage/              # SQLite 存储
├── proxy/                # 对外 HTTP 代理服务
├── webui/                # 登录页、仪表盘、API
├── logger/               # 内存日志 + stdout 输出
├── Dockerfile
└── docker-compose.yml
```

## 运行要求

- Go `1.25`
- 需要可用的 CGO 编译环境
  - 项目依赖 `github.com/mattn/go-sqlite3`
  - 本地构建通常需要 `gcc` / Xcode Command Line Tools

## 快速开始

### 本地运行

```bash
go run .
```

或先编译再启动：

```bash
go build -o proxy-pool .
./proxy-pool
```

程序启动后会：

1. 加载默认配置或读取 `config.json`
2. 初始化 SQLite 数据库
3. 启动 WebUI
4. 立即抓取一次代理并开始验证
5. 启动定时抓取和健康检查
6. 在 `:7777` 启动统一代理服务

### 默认端口

- 代理服务：`127.0.0.1:7777` 或 `:7777`
- WebUI：`http://127.0.0.1:7778`

### 使用聚合代理

例如：

```bash
curl -x http://127.0.0.1:7777 https://httpbin.org/ip
```

也可以给命令行程序设置环境变量：

```bash
export http_proxy=http://127.0.0.1:7777
export https_proxy=http://127.0.0.1:7777
```

## Docker

### 使用 Dockerfile

```bash
docker build -t proxygo .
docker run -d \
  --name proxygo \
  -p 127.0.0.1:7777:7777 \
  -p 7778:7778 \
  -e TZ=Asia/Shanghai \
  -e DATA_DIR=/app/data \
  -v "$(pwd)/data:/app/data" \
  proxygo
```

### 使用 docker-compose.yml

```bash
docker compose up -d --build
```

当前仓库中的 `docker-compose.yml` 有两个前提：

- 它会将 `./data` 挂载到容器内 `/app/data`
- 它依赖一个已存在的外部网络 `cursor2api_default`

如果宿主机没有这个网络，需要先创建，或者直接修改 `docker-compose.yml` 中的网络配置。

## 数据目录

程序支持通过 `DATA_DIR` 指定数据目录。

- 未设置 `DATA_DIR` 时：
  - 数据库默认写到项目根目录 `proxy.db`
  - 配置文件默认读取/写入项目根目录 `config.json`
- 设置 `DATA_DIR=/app/data` 时：
  - 数据库路径变为 `/app/data/proxy.db`
  - 配置文件路径变为 `/app/data/config.json`

## 配置说明

### 可持久化配置

当前版本只会从 `config.json` 读取并保存以下 4 个字段：

```json
{
  "fetch_interval": 30,
  "check_interval": 10,
  "validate_concurrency": 300,
  "validate_timeout": 3
}
```

字段含义：

- `fetch_interval`：定时抓取间隔，单位分钟
- `check_interval`：健康检查间隔，单位分钟
- `validate_concurrency`：并发验证数量
- `validate_timeout`：单个代理验证超时，单位秒

这些参数既可以通过编辑 `config.json` 修改，也可以在 WebUI 的“系统设置”中在线保存。

### 当前代码中的默认值

除上面 4 项外，其余配置目前来自代码默认值：

| 配置项 | 默认值 | 说明 |
| --- | --- | --- |
| `WebUIPort` | `:7778` | Web 管理后台端口 |
| `ProxyPort` | `:7777` | 对外统一代理端口 |
| `DBPath` | `proxy.db` 或 `${DATA_DIR}/proxy.db` | SQLite 数据库路径 |
| `ValidateURL` | `https://cursor.com/api/auth/me` | 验证目标地址 |
| `MaxResponseMs` | `2500` | 最大可接受延迟，毫秒 |
| `MaxFailCount` | `3` | 失败阈值字段已定义，但当前运行逻辑未完整使用 |
| `MaxRetry` | `3` | 请求失败后的重试次数 |

## WebUI

访问地址：

- `http://127.0.0.1:7778`

提供的功能：

- 登录鉴权
- 展示代理总数、HTTP 数量、SOCKS5 数量
- 按协议筛选代理
- 删除单个代理
- 手动触发抓取
- 查看最近日志
- 在线修改抓取/校验参数

### 登录密码说明

当前版本在代码里只保存了 WebUI 密码的 SHA256 哈希值，默认明文密码没有在仓库中说明，也不能通过 `config.json` 或 WebUI 修改。

如果你要自定义密码，当前可行方式是：

1. 生成密码的 SHA256
2. 修改 `config/config.go` 中的 `WebUIPasswordHash`
3. 重新构建并启动程序

例如生成 SHA256：

```bash
printf 'your-password' | shasum -a 256
```

## API 概览

除 `/login` 和 `/logout` 外，其余管理 API 都要求已登录会话。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/` | 仪表盘页面 |
| `GET/POST` | `/login` | 登录页面 / 提交登录 |
| `GET` | `/logout` | 退出登录 |
| `GET` | `/api/stats` | 代理统计信息 |
| `GET` | `/api/proxies?protocol=http` | 查询代理列表，可按协议筛选 |
| `POST` | `/api/proxy/delete` | 删除指定代理 |
| `POST` | `/api/fetch` | 手动触发一次抓取 |
| `GET` | `/api/logs` | 获取最近 200 条日志 |
| `GET/POST` | `/api/config` | 读取/保存运行参数 |

## 代理抓取与校验逻辑

当前实现会并发抓取内置代理源，然后做去重与验证：

- HTTP 源：`https://cdn.jsdelivr.net/gh/databay-labs/free-proxy-list/http.txt`
- SOCKS5 源：`https://cdn.jsdelivr.net/gh/databay-labs/free-proxy-list/socks5.txt`
- 混合源：`https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/all/data.txt`

验证规则：

- 仅接受 HTTP `200` 或 `204`
- 响应超时或延迟超过 `MaxResponseMs` 的代理会被丢弃
- 默认验证目标为 `https://cursor.com/api/auth/me`

## 日志

- 日志会输出到进程标准输出
- 同时会保留最近 500 条在内存中供 WebUI 展示
- `/api/logs` 当前返回最近 200 条日志

## 当前实现限制

- `config.Config` 中虽然定义了 `HTTPSourceURL` 和 `SOCKS5SourceURL`，但抓取器当前实际使用的是 `fetcher/defaultSources` 内置来源
- `config.json` 目前只持久化 4 个字段，不包含端口、密码哈希、验证 URL 等配置
- WebUI 登录密码不能在线修改
- 代理请求失败时，运行逻辑倾向于直接删除上游代理，`MaxFailCount` 目前没有完整参与主流程
- 日志没有单独写文件，管理端看到的是内存中的最近日志窗口

## 适用场景

- 在本机快速聚合一批公开代理，提供给命令行或程序统一使用
- 临时验证免费 HTTP / SOCKS5 代理的可用性
- 通过简单 Web 面板查看当前代理池状态

如果后续要继续完善，优先建议补这几项：

- 支持通过配置文件完整覆盖所有默认参数
- 支持自定义代理源并真正接入抓取器
- 支持 WebUI 密码初始化和修改
- 为失败计数、重试和删除策略补齐一致的状态流转
- 增加自动化测试
