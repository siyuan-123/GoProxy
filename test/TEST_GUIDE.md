# GoProxy 测试指南

本项目提供了三种测试脚本，用于验证代理服务的功能和性能。所有脚本都采用**持续运行模式**（类似 `ping` 命令），按 `Ctrl+C` 停止并显示统计信息。

## 📋 测试脚本说明

所有测试脚本位于 `test/` 目录下。

### 1. Bash 脚本（推荐）- `test_proxy.sh`

最简单直接，使用 `curl` 命令持续测试代理服务。

**使用方法：**

```bash
# 测试随机轮换模式（默认 7777 端口）
./test/test_proxy.sh

# 测试最低延迟模式（7776 端口）
./test/test_proxy.sh 7776

# 自定义端口
./test/test_proxy.sh 8080
```

**特点：**
- 无需安装依赖（仅需 curl 和 Python3）
- 兼容 macOS 和 Linux
- 持续运行，按 Ctrl+C 停止
- 显示国旗 emoji 和出口 IP

---

### 2. Go 脚本 - `test_proxy.go`

使用 Go 语言编写，与项目本身技术栈一致。

**使用方法：**

```bash
# 测试随机轮换模式（默认 7777 端口）
go run test/test_proxy.go

# 测试最低延迟模式（7776 端口）
go run test/test_proxy.go 7776

# 自定义端口
go run test/test_proxy.go 8080

# 或编译后运行
cd test && go build -o test_proxy test_proxy.go
./test_proxy 7776
```

**特点：**
- 与项目技术栈统一
- 可编译为独立二进制
- 显示国旗 emoji 和出口 IP
- 持续运行，按 Ctrl+C 停止
- 彩色输出

---

### 3. Python 脚本 - `test_proxy.py`

使用 Python 编写，语法简洁易读。

**使用方法：**

```bash
# 安装依赖
pip install requests

# 测试随机轮换模式（默认 7777 端口）
python test/test_proxy.py

# 测试最低延迟模式（7776 端口）
python test/test_proxy.py 7776

# 自定义端口
python test/test_proxy.py 8080
```

**特点：**
- Python 生态丰富
- 易于扩展和修改
- 持续运行，按 Ctrl+C 停止
- 显示国旗 emoji 和出口 IP

---

## 🎯 测试输出示例

```
PROXY 127.0.0.1:7777 (http://ip-api.com/json/?fields=countryCode,query): continuous mode

proxy from 🇺🇸 203.0.113.45: seq=1 time=1234ms
proxy from 🇩🇪 198.51.100.78: seq=2 time=987ms
proxy from 🇬🇧 192.0.2.123: seq=3 time=1567ms
proxy from 🇺🇸 203.0.113.45: seq=4 time=1123ms
proxy #5: request failed (timeout)
proxy from 🇯🇵 198.51.100.12: seq=6 time=890ms
proxy from 🇫🇷 192.0.2.234: seq=7 time=1456ms
proxy from 🇨🇳 68.71.249.153: seq=8 time=1102ms
...
^C
---
50 requests transmitted, 47 received, 3 failed, 6.0% packet loss
```

**特点：**
- ✅ 简洁输出，类似 `ping` 命令
- ✅ 一行一个结果，清晰易读
- ✅ 显示国旗 emoji、出口 IP 和延迟
- ✅ 按 Ctrl+C 停止并显示统计摘要
- ✅ 显示丢包率（失败率）

## 🔍 观察要点

### 1. 出口 IP 变化
- 每次请求的出口 IP **应该不同**，证明代理池在轮换
- 如果连续多次是同一个 IP，说明池子中可用代理较少
- 持续运行可以观察到代理的循环使用模式

### 2. 延迟表现
- 延迟应该在合理范围内（通常 < 2000ms）
- 如果延迟过高或超时，说明代理质量下降
- 观察延迟趋势：是否稳定在某个范围

### 3. 成功率（动态显示）
- 正常情况下成功率应该 > 90%
- 持续运行时成功率会动态更新
- 如果成功率持续下降，需要检查：
  - 代理池是否有足够的健康代理
  - 网络连接是否正常
  - 代理源质量是否下降

### 4. 协议对比
- HTTP 代理：速度通常较快，兼容性好
- SOCKS5 代理：更底层，可传输各种协议数据

### 5. 持续测试优势
- 可以长时间运行，观察代理池的稳定性
- 实时监控成功率变化
- 按 Ctrl+C 随时查看统计结果
- 类似 `ping` 命令的使用体验

## 📊 配合 WebUI 监控

测试时可以同时打开 WebUI (http://localhost:7778)：

1. **代理列表**：查看正在使用的代理
2. **使用统计**：观察 `使用次数` 列的变化
3. **系统日志**：实时查看代理请求日志
4. **质量分布**：查看当前池子的质量分布

## ⚙️ 自定义配置

修改脚本顶部的配置变量：

```bash
PROXY_HOST="127.0.0.1"              # 代理服务地址
PROXY_PORT="7777"                    # 代理服务端口
TEST_URL="http://httpbin.org/ip"    # 测试目标 URL
DELAY=1                              # 每次请求间隔（秒）
```

**注意**：所有脚本现在都是持续运行模式，按 `Ctrl+C` 停止，无需配置请求次数。

## 🛠️ 高级测试

### 测试特定网站

```bash
# 测试访问 Google
curl -x "http://127.0.0.1:7777" https://www.google.com

# 测试访问 GitHub API
curl -x "http://127.0.0.1:7777" https://api.github.com/users/github

# 测试中国网站
curl -x "http://127.0.0.1:7777" https://www.baidu.com
```

### 并发压力测试

```bash
# 使用 ab (Apache Bench) 进行压力测试
ab -n 100 -c 10 -X 127.0.0.1:7777 http://httpbin.org/ip

# 使用 wrk 进行压力测试
wrk -t4 -c100 -d30s --proxy http://127.0.0.1:7777 http://httpbin.org/ip
```

### 查看当前使用的代理

```bash
# 通过 httpbin 获取当前出口 IP
curl -x "http://127.0.0.1:7777" http://httpbin.org/ip

# 获取更详细的信息（包括 headers）
curl -x "http://127.0.0.1:7777" http://httpbin.org/anything
```

## 📝 注意事项

1. **启动代理服务**：测试前确保 `goproxy` 已启动
   ```bash
   ./goproxy
   ```

2. **等待代理池就绪**：首次启动需要等待代理抓取和验证（约 30-60 秒）

3. **网络环境**：确保服务器可以访问外部代理源和测试 URL

4. **防火墙**：确保 7777 端口没有被防火墙阻止

5. **代理协议**：不同协议可能表现不同，建议都测试一遍

## 🐛 故障排查

### 全部请求失败
- 检查 `goproxy` 服务是否正在运行
- 检查代理池是否有可用代理（访问 WebUI 查看）
- 检查端口 7777 是否被占用

### 成功率低
- 查看 WebUI 日志，确认代理质量
- 可能需要等待池子优化（系统会自动轮换低质量代理）
- 检查网络连接是否稳定

### SOCKS5 测试失败
- 确认 curl 版本支持 SOCKS5（`curl --version`）
- Python 脚本需要安装 `pysocks`：`pip install pysocks`
- Go 脚本需要安装依赖：`go get golang.org/x/net/proxy`

### Bash 脚本时间戳错误（macOS）
如果看到 `value too great for base` 错误：
- 脚本已自动使用 Python3 获取毫秒时间戳（macOS 兼容）
- 确保系统已安装 Python3（macOS 自带）
- 或安装 GNU coreutils：`brew install coreutils`

---

**Happy Testing! 🚀**
