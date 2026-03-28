# GoProxy

> **智能代理池系统** — 基于 Go 的轻量级、低资源消耗、自适应的代理池服务

goproxy 从多个公开代理源自动抓取 HTTP/SOCKS5 代理，通过严格验证（出口 IP + 位置 + 延迟）后加入智能代理池，对外提供统一的 HTTP 代理服务。系统采用质量分级、智能补充、自动优化等机制，确保代理池始终保持高质量和稳定性。

## ✨ 核心特性

### 🎯 智能池子机制
- **固定容量管理**：可配置池子大小和 HTTP/SOCKS5 协议比例
- **质量分级**：S/A/B/C 四级评分（基于延迟），智能选择高质量代理
- **动态状态感知**：Healthy → Warning → Critical → Emergency 四级状态自适应
- **严格准入标准**：必须通过出口 IP、地理位置、延迟三重验证才可入池
- **智能替换**：新代理必须显著优于现有代理（默认快 30%）才触发替换

### 🚀 按需抓取
- **源分组策略**：快更新源（5-30min）用于紧急补充，慢更新源（每天）用于优化轮换
- **断路器保护**：连续失败的源自动降级/禁用，冷却后恢复
- **多模式抓取**：
  - **Emergency**：单协议缺失或池子 <10%，使用所有可用源
  - **Refill**：池子 <80%，使用快更新源
  - **Optimize**：池子健康时，随机抽取少量慢源优化替换

### 🏥 分层健康管理
- **轻量批次检查**：每次仅检查 20 个代理，避免资源浪费
- **智能跳过 S 级**：池子健康时跳过 S 级代理检查
- **定时优化轮换**：健康状态下，定期抓取优质代理替换池中延迟高的

### 🔄 智能重试机制
- **自动故障切换**：代理请求失败时立即切换到另一个代理重试（最多 3 次）
- **失败即删除**：连接失败或请求超时的代理立即从池子中移除
- **用户无感知**：自动重试在服务端完成，用户只会收到成功响应或最终失败提示
- **防重复尝试**：已尝试过的失败代理不会在同一请求中再次使用

### 🚪 双端口策略
- **7777 端口（随机轮换）**：每次请求随机选择代理，IP 高度分散，适合爬虫和数据采集
- **7776 端口（最低延迟）**：固定使用延迟最低的代理，除非失败才切换，适合长连接和流媒体
- **自动切换**：两个端口都支持失败自动重试，7776 失败后切换到次优代理
- **共享池子**：两个端口使用同一个代理池，统一管理和优化

### 🎨 黑客风格 WebUI
- **Matrix 美学**：荧光绿 + 纯黑背景，CRT 扫描线效果，JetBrains Mono 等宽字体
- **双角色权限**：访客模式（只读）+ 管理员模式（完全控制），可安全公网开放
- **实时仪表盘**：池子状态、质量分布可视化、协议统计，带荧光光晕效果
- **完整配置界面**：池子容量、延迟标准、验证参数、优化策略均可在线调整（管理员）
- **代理注册表**：详细展示地址、出口 IP、位置、延迟、质量等级、使用统计
- **中英文切换**：支持中文/英文界面切换，默认中文
- **交互优化**：点击地址复制、单个代理刷新、实时日志倒计时

### 📊 适用场景
- **小型 VPS**：低资源消耗（固定池子 + 按需抓取 + 限流查询）
- **稳定需求**：自动剔除失败代理，始终保持健康池子
- **质量优先**：S/A 级代理优先使用，自动优化延迟

## 📦 项目结构

```text
.
├── main.go               # 程序入口，协调所有模块
├── config/               # 配置系统（池子容量、延迟标准、验证参数等）
├── pool/                 # 🆕 池子管理器（入池判断、替换逻辑、状态计算）
├── fetcher/              # 🆕 智能抓取器（源分组、断路器、按需抓取）
│   ├── fetcher.go        # 多模式抓取逻辑
│   ├── source_manager.go # 源状态管理和断路器
│   └── ip_query.go       # IP查询限流和多源降级
├── validator/            # 代理验证（连接测试 + 出口IP检测）
├── checker/              # 🆕 分批健康检查器
├── optimizer/            # 🆕 优化轮换器（定时优化池子质量）
├── storage/              # 🆕 扩展存储层（质量等级、使用统计、源状态表）
├── proxy/                # 对外 HTTP 代理服务
├── webui/                # 🆕 黑客风格 WebUI（健康仪表盘、配置界面）
├── logger/               # 内存日志收集
├── test/                 # 🧪 测试脚本（Bash/Go/Python）
│   ├── test_proxy.sh     # Bash 测试脚本
│   ├── test_proxy.go     # Go 测试脚本
│   ├── test_proxy.py     # Python 测试脚本
│   └── TEST_GUIDE.md     # 测试指南
└── POOL_DESIGN.md        # 🆕 完整架构设计文档
```

## 🚀 快速开始

### 运行要求
- Go `1.25`
- CGO 编译环境（依赖 `github.com/mattn/go-sqlite3`）

### 本地运行

```bash
go run .
```

或先编译再启动：

```bash
go build -o proxygo .
./proxygo
```

程序启动后会：
1. 加载配置（优先 `config.json`）
2. 初始化数据库和限流器
3. 启动 WebUI（`:7778`）
4. 立即执行智能填充（按需抓取 + 严格验证）
5. 启动后台协程：
   - 状态监控（每分钟）
   - 健康检查（默认 5 分钟）
   - 优化轮换（默认 30 分钟）
6. 启动两个代理服务：
   - `:7776` - 最低延迟模式（稳定连接）
   - `:7777` - 随机轮换模式（IP 多样性）

### 默认端口
- **代理服务（随机轮换）**：`127.0.0.1:7777` - 每次请求随机选择代理，IP 多样性高
- **代理服务（最低延迟）**：`127.0.0.1:7776` - 固定使用延迟最低的代理，性能优先
- **WebUI**：`http://127.0.0.1:7778`
- **默认密码**：`goproxy`（可通过 `WEBUI_PASSWORD` 环境变量自定义）

### 使用代理

GoProxy 提供**两个代理端口**，满足不同场景需求：

#### 🎲 7777 端口 - 随机轮换模式

适合需要 **IP 多样性** 的场景（爬虫、数据采集、负载均衡）：

```bash
curl -x http://127.0.0.1:7777 https://httpbin.org/ip
```

**特点**：
- 每次请求随机选择一个代理
- 优先使用高质量（S/A 级）代理
- IP 地址高度分散

#### ⚡ 7776 端口 - 最低延迟模式

适合需要 **稳定连接** 的场景（长连接、流媒体、实时通信）：

```bash
curl -x http://127.0.0.1:7776 https://httpbin.org/ip
```

**特点**：
- 固定使用池中延迟最低的代理
- 除非该代理失败，否则不会切换
- 失败时自动删除并切换到下一个最低延迟代理
- 性能和稳定性最优

#### 环境变量配置

```bash
# 使用随机模式
export http_proxy=http://127.0.0.1:7777
export https_proxy=http://127.0.0.1:7777

# 或使用稳定模式
export http_proxy=http://127.0.0.1:7776
export https_proxy=http://127.0.0.1:7776
```

#### 端口对比

| 特性 | 7777（随机轮换） | 7776（最低延迟） |
|------|-----------------|-----------------|
| **选择策略** | 随机选择（优先高质量） | 固定使用延迟最低的 |
| **IP 多样性** | ⭐⭐⭐⭐⭐ 高度分散 | ⭐ 基本固定 |
| **连接稳定性** | ⭐⭐⭐ 每次切换 | ⭐⭐⭐⭐⭐ 固定不变 |
| **性能表现** | ⭐⭐⭐⭐ 平均延迟 | ⭐⭐⭐⭐⭐ 最优延迟 |
| **适用场景** | 爬虫、数据采集、防封禁 | 长连接、流媒体、下载 |
| **失败切换** | 自动重试 3 次 | 失败后切换到次优代理 |

#### 自动重试机制说明

当你通过 GoProxy 发送请求时，如果上游代理失败，系统会**自动处理**：

1. **立即删除失败代理**：从池子中移除不可用的代理
2. **自动切换重试**：随机选择另一个可用代理重新发送请求（最多重试 3 次）
3. **用户完全无感知**：整个过程在服务端完成，你的应用只会收到成功响应或最终失败提示
4. **防止重复尝试**：同一请求中不会重复使用已失败的代理

**示例流程**：
```
用户请求 → 代理A失败(删除) → 自动切换代理B → 代理B成功 → 返回响应
```

这意味着即使池子中有部分失效代理，你的应用依然可以正常工作，系统会自动保持池子质量。

## 🐳 Docker 部署

### 使用 Dockerfile

```bash
docker build -t proxygo .
docker run -d \
  --name proxygo \
  -p 127.0.0.1:7776:7776 \
  -p 127.0.0.1:7777:7777 \
  -p 7778:7778 \
  -e TZ=Asia/Shanghai \
  -e WEBUI_PASSWORD=your_password \
  -e DATA_DIR=/app/data \
  -v "$(pwd)/data:/app/data" \
  proxygo
```

### 使用 docker-compose

```bash
docker compose up -d --build
```

### WebUI 公网访问配置

得益于**双角色权限系统**，WebUI 可以安全地对外开放：

```bash
docker run -d \
  --name proxygo \
  -p 127.0.0.1:7776:7776 \
  -p 127.0.0.1:7777:7777 \
  -p 0.0.0.0:7778:7778 \    # 公网访问 WebUI
  -e TZ=Asia/Shanghai \
  -e WEBUI_PASSWORD=strong_password \
  -e DATA_DIR=/app/data \
  -v "$(pwd)/data:/app/data" \
  proxygo
```

**安全说明**：
- 访客（未登录）只能查看数据，无法执行任何操作
- 所有写操作（抓取、删除、配置修改）都需要管理员密码
- 建议设置强密码（通过 `WEBUI_PASSWORD` 环境变量）
- 代理服务端口（7776、7777）建议仅绑定内网（`127.0.0.1`）

## ⚙️ 配置说明

### 配置文件示例

所有配置均可通过 WebUI 的 **Configure Pool** 界面在线调整，也可以手动编辑 `config.json`：

```json
{
  "pool_max_size": 100,
  "pool_http_ratio": 0.5,
  "pool_min_per_protocol": 10,
  "max_latency_ms": 2000,
  "max_latency_healthy": 1500,
  "max_latency_emergency": 3000,
  "validate_concurrency": 300,
  "validate_timeout": 8,
  "health_check_interval": 5,
  "health_check_batch_size": 20,
  "optimize_interval": 30,
  "replace_threshold": 0.7
}
```

### 配置参数详解

**服务端口配置**

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `proxy_port` | `:7777` | 随机轮换代理端口 |
| `stable_proxy_port` | `:7776` | 最低延迟代理端口 |
| `webui_port` | `:7778` | WebUI 端口 |

**池子容量配置**

| 参数 | 默认值 | 说明 | 推荐范围 |
| --- | --- | --- | --- |
| `pool_max_size` | `100` | 代理池总容量 | 50-500 |
| `pool_http_ratio` | `0.5` | HTTP 协议占比 | 0.3-0.8 |
| `pool_min_per_protocol` | `10` | 每协议最少保证数量 | 5-50 |

**延迟标准配置**

| 参数 | 默认值 | 说明 | 推荐范围 |
| --- | --- | --- | --- |
| `max_latency_ms` | `2000` | 标准模式最大延迟（毫秒） | 1000-3000 |
| `max_latency_healthy` | `1500` | 健康模式严格延迟（毫秒） | 800-2000 |
| `max_latency_emergency` | `3000` | 紧急模式放宽延迟（毫秒） | 2000-5000 |

**验证与健康检查配置**

| 参数 | 默认值 | 说明 | 推荐范围 |
| --- | --- | --- | --- |
| `validate_concurrency` | `300` | 并发验证数量 | 100-500 |
| `validate_timeout` | `8` | 验证超时（秒） | 5-15 |
| `health_check_interval` | `5` | 检查间隔（分钟） | 3-15 |
| `health_check_batch_size` | `20` | 每批检查数量 | 10-50 |

**优化配置**

| 参数 | 默认值 | 说明 | 推荐范围 |
| --- | --- | --- | --- |
| `optimize_interval` | `30` | 优化轮换间隔（分钟） | 15-120 |
| `replace_threshold` | `0.7` | 替换阈值（新代理需快 30%） | 0.5-0.9 |

### 不同场景配置建议

**小型 VPS（1C2G）**
```json
{
  "pool_max_size": 50,
  "pool_http_ratio": 0.5,
  "validate_concurrency": 100,
  "health_check_interval": 10,
  "health_check_batch_size": 10,
  "optimize_interval": 60
}
```

**中型服务器（2C4G+）**
```json
{
  "pool_max_size": 200,
  "pool_http_ratio": 0.6,
  "validate_concurrency": 300,
  "health_check_interval": 5,
  "health_check_batch_size": 30,
  "optimize_interval": 30
}
```

**低延迟优先**
```json
{
  "pool_max_size": 100,
  "max_latency_ms": 1000,
  "max_latency_healthy": 800,
  "optimize_interval": 15,
  "replace_threshold": 0.8
}
```

**高可用优先（需要更多代理）**
```json
{
  "pool_max_size": 300,
  "pool_http_ratio": 0.7,
  "pool_min_per_protocol": 20,
  "max_latency_ms": 3000
}
```

### 固定配置

以下配置在代码中固定，无需调整：

| 配置项 | 值 | 说明 |
| --- | --- | --- |
| `WebUIPort` | `:7778` | Web 管理后台端口 |
| `ProxyPort` | `:7777` | 对外统一代理端口 |
| `ValidateURL` | `http://www.gstatic.com/generate_204` | 验证目标地址 |
| `IPQueryRateLimit` | `10` | IP 查询限流（次/秒） |
| `SourceFailThreshold` | `3` | 源降级阈值 |
| `SourceDisableThreshold` | `5` | 源禁用阈值 |
| `SourceCooldownMinutes` | `30` | 源禁用冷却时间（分钟） |

## 🎨 WebUI 使用指南

访问地址：`http://127.0.0.1:7778`

### 👥 双角色权限系统

GoProxy WebUI 支持**访客模式**和**管理员模式**：

#### 访客模式（只读）

**无需登录**即可访问，可以查看所有数据但不能操作：

- ✅ 查看池子状态和质量分布
- ✅ 查看代理列表和详细信息
- ✅ 查看系统日志
- ✅ 点击复制代理地址
- ❌ 不能抓取代理
- ❌ 不能刷新延迟
- ❌ 不能删除代理
- ❌ 不能修改配置

**适用场景**：
- 团队成员监控代理池状态
- 展示给客户或第三方查看
- 公网开放访问（只读数据安全）

#### ⚡ 管理员模式（完全控制）

**登录后**拥有所有操作权限：

- ✅ 所有访客模式的查看功能
- ✅ 手动触发代理抓取
- ✅ 刷新所有代理延迟
- ✅ 刷新单个代理信息
- ✅ 删除指定代理
- ✅ 修改池子配置（容量、延迟标准、检查间隔等）

**默认密码**：`goproxy`（通过环境变量 `WEBUI_PASSWORD` 自定义）

### 健康仪表盘

**四宫格指标卡**
- **Pool Status**：当前池子状态（HEALTHY/WARNING/CRITICAL/EMERGENCY）
- **Total Proxies**：总代理数 / 池子容量
- **HTTP**：HTTP 代理数 / HTTP 槽位数 + 平均延迟
- **SOCKS5**：SOCKS5 代理数 / SOCKS5 槽位数 + 平均延迟

**质量分布可视化**
- 横向条形图展示 S/A/B/C 四级质量分布
- 实时显示各级别代理数量

### 代理注册表

**表格字段**
- **Grade**：质量等级（S/A/B/C）
- **Protocol**：协议类型（HTTP/SOCKS5）
- **Address**：代理地址（host:port）
- **Exit IP**：出口 IP
- **Location**：出口地理位置（国旗 + 国家代码 + 城市）
- **Latency**：延迟（毫秒，颜色编码）
- **Usage**：使用次数 / 成功次数
- **Action**：删除按钮

**操作功能**
- **筛选**：All / HTTP / SOCKS5（所有用户可用）
- **点击复制地址**：点击代理地址单元格复制到剪贴板（所有用户可用）
- **Fetch Proxies**：手动触发智能抓取（⚡ 管理员专属）
- **Refresh Latency**：重新验证所有代理并更新延迟（⚡ 管理员专属）
- **刷新单个代理**：点击行内刷新按钮验证单个代理（⚡ 管理员专属）
- **删除代理**：点击行内删除按钮移除指定代理（⚡ 管理员专属）
- **Configure Pool**：打开配置界面（⚡ 管理员专属）

### 配置界面（⚡ 管理员专属）

点击 **Configure Pool** 打开配置模态框，包含：

**Pool Capacity 部分**
- Max Size：池子总容量
- HTTP Ratio：HTTP 协议占比（0.5 = 50%）
- Min Per Protocol：每协议最小保证

**Latency Standards 部分**
- Standard：标准模式延迟阈值
- Healthy：健康模式严格延迟
- Emergency：紧急模式放宽延迟

**Validation & Health Check 部分**
- Validate Concurrency：并发验证数
- Validate Timeout：验证超时
- Health Check Interval：健康检查间隔
- Health Check Batch Size：每批检查数量

**Optimization 部分**
- Optimize Interval：优化轮换间隔
- Replace Threshold：替换阈值（0.7 = 新代理需快 30%）

保存后立即生效，系统会自动调整池子策略。

## 🏗️ 核心架构

### 智能池子生命周期

```text
[启动] → 状态监控 → 判断池子健康度
              ↓
         需要补充？
          ↙     ↘
        是        否
         ↓         ↓
    智能抓取    保持监控
    (多模式)       ↓
         ↓      优化轮换
    严格验证   (定时执行)
         ↓         ↓
    智能入池    替换劣质代理
    (替换逻辑)     ↓
         ↓    分批健康检查
         ↓    (剔除失败)
         ↓         ↓
         └─────────┘
              ↓
         持续优化循环
```

### 状态转换机制

```text
Healthy (总数≥80% 且 各协议≥80%槽位)
   ↓ 代理失效
Warning (总数<80% 或 任一协议<80%)
   ↓ 继续失效
Critical (总数<50% 或 任一协议<20%槽位)
   ↓ 继续失效
Emergency (总数<10% 或 单协议缺失)
   ↑
   └─ 自动触发紧急抓取 ─┘
```

### 抓取模式选择

| 池子状态 | 抓取模式 | 使用源 | 触发条件 |
| --- | --- | --- | --- |
| Emergency | 紧急模式 | 所有可用源 | 单协议缺失或总数<10% |
| Critical/Warning | 补充模式 | 快更新源 | 总数<80%或协议不均 |
| Healthy | 优化模式 | 慢更新源（随机2-3个） | 定时触发（30分钟） |

### 质量分级标准

| 等级 | 延迟范围 | 说明 | 权重 |
| --- | --- | --- | --- |
| S | ≤500ms | 超快，优先使用，健康状态跳过检查 | 最高 |
| A | 501-1000ms | 良好，稳定可用 | 高 |
| B | 1001-2000ms | 可用，会被优化替换 | 中 |
| C | >2000ms | 淘汰候选，优先替换 | 低 |

## 🔧 数据库 Schema

### proxies 表

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | INTEGER | 主键 |
| `address` | TEXT | 代理地址（UNIQUE） |
| `protocol` | TEXT | 协议类型（http/socks5） |
| `exit_ip` | TEXT | 出口 IP |
| `exit_location` | TEXT | 出口位置 |
| `latency` | INTEGER | 延迟（毫秒） |
| `quality_grade` | TEXT | 质量等级（S/A/B/C） |
| `use_count` | INTEGER | 使用次数 |
| `success_count` | INTEGER | 成功次数 |
| `fail_count` | INTEGER | 失败次数 |
| `last_used` | DATETIME | 最后使用时间 |
| `last_check` | DATETIME | 最后检查时间 |
| `created_at` | DATETIME | 创建时间 |
| `status` | TEXT | 状态（active/degraded/candidate_replace） |

### source_status 表

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | INTEGER | 主键 |
| `url` | TEXT | 源地址（UNIQUE） |
| `success_count` | INTEGER | 成功次数 |
| `fail_count` | INTEGER | 失败次数 |
| `consecutive_fails` | INTEGER | 连续失败次数 |
| `last_success` | DATETIME | 最后成功时间 |
| `last_fail` | DATETIME | 最后失败时间 |
| `status` | TEXT | 状态（active/degraded/disabled） |
| `disabled_until` | DATETIME | 禁用到期时间 |

## 🔍 代理源

系统内置 16 个代理源，分为快更新和慢更新两组：

**快更新源（5-30分钟更新）**
- proxifly/free-proxy-list (HTTP/SOCKS4/SOCKS5)
- ProxyScraper/ProxyScraper (HTTP/SOCKS4/SOCKS5)
- monosans/proxy-list (HTTP)

**慢更新源（每天更新）**
- TheSpeedX/SOCKS-List (HTTP/SOCKS4/SOCKS5)
- monosans/proxy-list (SOCKS4/SOCKS5)
- databay-labs/free-proxy-list (HTTP/SOCKS5)

系统会根据池子状态自动选择合适的源组：
- 紧急/补充模式：使用快更新源，快速填充
- 优化模式：随机选择慢更新源，精细优化

## 🚦 核心机制详解

### 1. 智能入池机制

每个代理在入池前需通过：
1. **连接验证**：能否成功连接 `http://www.gstatic.com/generate_204`
2. **出口 IP 检测**：获取代理的出口 IP
3. **地理位置查询**：获取出口 IP 的国家/城市
4. **延迟测试**：测量连接延迟
5. **质量评估**：根据延迟计算质量等级

**入池判断逻辑**
- ✅ 协议槽位未满：直接加入
- ✅ 槽位满但总量允许10%浮动：浮动加入
- 🔄 池子满且质量更优：替换延迟最高的现有代理（需快30%+）
- ❌ 池子满且质量不足：拒绝

### 2. 健康检查机制

**批次检查策略**
- 每次检查 20 个代理（可配置）
- 优先检查长时间未检查的
- 池子健康时跳过 S 级代理（降低资源消耗）

**检查结果处理**
- ✅ 验证通过：更新延迟、出口 IP、质量等级
- ❌ 验证失败：失败计数 +1，≥3次自动删除

### 3. 优化轮换机制

**触发条件**
- 池子状态：Healthy
- 定时触发：默认 30 分钟

**优化流程**
1. 从慢更新源随机抽取 2-3 个源
2. 抓取候选代理并验证
3. 筛选出延迟 ≤1500ms 的优质代理
4. 尝试替换池中 B/C 级代理（需快30%+）

**资源控制**
- 仅在池子健康时执行
- 抽取少量源，避免浪费
- 严格质量标准（≤1500ms）

### 4. 源管理与断路器

**状态跟踪**
- 记录每个源的成功/失败次数
- 连续失败 3 次：降级（Degraded）
- 连续失败 5 次：禁用 30 分钟（Disabled）
- 冷却期结束：自动恢复为 Active

**好处**
- 避免浪费资源在失效源上
- 自动恢复，无需人工干预
- 保护系统免受源故障影响

## 📖 常见问题

### Q: 为什么池子容量是固定的？
A: 固定容量可以：
- **可预测资源消耗**：内存、CPU、网络带宽均可控
- **提升代理质量**：通过严格准入和替换保持高质量
- **简化管理逻辑**：避免无限增长和复杂的淘汰策略

### Q: 如何调整池子大小和协议比例？
A: 
1. 访问 WebUI → 点击 **Configure Pool**
2. 修改 **Max Size** 和 **HTTP Ratio**
3. 点击 **Save Configuration**
4. 系统会自动调整槽位分配

示例：
- 池子大小 200，HTTP 比例 0.7 → HTTP 槽位 140，SOCKS5 槽位 60
- 池子大小 50，HTTP 比例 0.3 → HTTP 槽位 15，SOCKS5 槽位 35

### Q: 池子状态如何计算？
A: 
- **Healthy**：总数 ≥80% 且各协议 ≥80% 槽位
- **Warning**：总数 <80% 或任一协议 <80% 槽位
- **Critical**：总数 <50% 或任一协议 <20% 槽位
- **Emergency**：总数 <10% 或单协议缺失

### Q: 如何优化延迟？
A: 系统会自动优化，也可以手动调整：
1. 降低 `max_latency_healthy`（严格模式）
2. 增加 `optimize_interval` 频率（更频繁优化）
3. 调高 `replace_threshold`（要求新代理更快）
4. 点击 **Refresh Latency** 立即重新验证

### Q: 为什么有的代理没有出口 IP？
A: 
- IP 查询有限流（10 次/秒）
- 部分代理可能不支持 IP 查询
- 系统会在后续健康检查中补全信息

### Q: 资源消耗如何？
A: 
- **内存**：池子 100 个约 50MB，200 个约 100MB
- **CPU**：空闲时 <1%，验证时 10-30%（取决于并发数）
- **网络**：
  - IP 查询限流 10 次/秒
  - 按需抓取，避免无效流量
  - 健康检查批次小（20 个）

## 📚 详细设计文档

完整的架构设计、模块说明、配置策略、资源优化方案，请查看：

👉 [POOL_DESIGN.md](./POOL_DESIGN.md)

## 🛠️ 开发与调试

### 查看日志

日志会输出到 stdout，同时在 WebUI 的 **System Log** 部分实时展示。

关键日志标识：
- `[pool]`：池子管理器
- `[fetch]`：抓取器
- `[source]`：源管理器
- `[health]`：健康检查器
- `[optimize]`：优化器
- `[monitor]`：状态监控器

### 数据库操作

```bash
# 查看当前代理
sqlite3 data/proxy.db "SELECT address, protocol, latency, quality_grade, status FROM proxies LIMIT 10;"

# 查看质量分布
sqlite3 data/proxy.db "SELECT quality_grade, COUNT(*) FROM proxies WHERE status='active' GROUP BY quality_grade;"

# 查看源状态
sqlite3 data/proxy.db "SELECT url, status, consecutive_fails FROM source_status;"

# 清空池子（慎用）
sqlite3 data/proxy.db "DELETE FROM proxies;"
```

## 🧪 测试代理服务

项目提供了三种测试脚本，用于验证代理服务功能和性能（位于 `test/` 目录）。

### 快速测试

```bash
# 测试随机轮换模式（默认 7777 端口）
./test/test_proxy.sh

# 测试最低延迟模式（7776 端口）
./test/test_proxy.sh 7776

# 使用 Go 脚本
go run test/test_proxy.go          # 默认 7777
go run test/test_proxy.go 7776     # 测试 7776

# 使用 Python 脚本
python test/test_proxy.py          # 默认 7777
python test/test_proxy.py 7776     # 测试 7776

# 按 Ctrl+C 停止测试并查看统计
```

测试脚本特点：
- **持续运行模式**：类似 `ping` 命令，持续发送请求
- 实时显示每次请求的出口 IP 和延迟
- 动态更新成功率统计
- 验证代理轮换机制
- 按 `Ctrl+C` 停止并显示完整统计报告

### 测试输出示例

```
PROXY 127.0.0.1:7777 (http://ip-api.com/json/?fields=countryCode,query): continuous mode

proxy from 🇺🇸 203.0.113.45: seq=1 time=1234ms
proxy from 🇩🇪 198.51.100.78: seq=2 time=987ms
proxy from 🇬🇧 192.0.2.123: seq=3 time=1567ms
proxy #4: request failed (timeout)
proxy from 🇯🇵 198.51.100.12: seq=5 time=890ms
...（持续运行，按 Ctrl+C 停止）

^C
---
50 requests transmitted, 47 received, 3 failed, 6.0% packet loss
```

详细测试指南请查看：👉 [test/TEST_GUIDE.md](./test/TEST_GUIDE.md)

## 🙏 致谢与声明

本项目基于 [jonasen1988/proxygo](https://github.com/jonasen1988/proxygo) 进行魔改和增强。

### 原项目
- **项目地址**：https://github.com/jonasen1988/proxygo
- **作者**：jonasen1988
- **基础功能**：代理抓取、验证、存储、HTTP代理服务、WebUI管理

### 本项目增强功能
在原项目基础上，我们进行了大量改进和功能增强：

- 🆕 **智能池子机制**：固定容量管理、质量分级（S/A/B/C）、智能替换逻辑
- 🆕 **按需抓取策略**：源分组、断路器保护、Emergency/Refill/Optimize 多模式
- 🆕 **分层健康管理**：批次检查、智能跳过 S 级、定时优化轮换
- 🆕 **智能重试机制**：自动故障切换、失败即删除、防重复尝试
- 🆕 **双端口服务**：7777 随机轮换（IP 多样性）+ 7776 最低延迟（稳定连接）
- 🆕 **黑客风格 WebUI**：Matrix 美学、实时仪表盘、完整配置界面、中英文切换
- 🆕 **双角色权限**：访客模式（只读）+ 管理员模式（完全控制），可安全公网开放
- 🆕 **扩展存储层**：质量等级、使用统计、源状态管理
- 🆕 **测试套件**：Bash/Go/Python 三种测试脚本，持续运行模式，显示国旗 emoji

感谢原作者提供的基础实现，让我们能够在此之上构建更强大的代理池系统。

同时感谢 [LINUX DO](https://linux.do/) 社区的支持。

## 📝 License

MIT License