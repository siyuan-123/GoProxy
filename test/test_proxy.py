#!/usr/bin/env python3
"""
GoProxy 持续测试脚本 - 类似 ping 命令的简洁输出
按 Ctrl+C 停止测试
"""

import requests
import time
import sys
import signal
from requests.exceptions import RequestException

# 配置
PROXY_HOST = "127.0.0.1"
PROXY_PORT = int(sys.argv[1]) if len(sys.argv) > 1 and sys.argv[1].isdigit() else 7777
TEST_URL = "http://ip-api.com/json/?fields=countryCode,query"
DELAY_SECONDS = 1

# 统计变量
total_count = 0
success_count = 0


def country_to_emoji(country_code):
    """将国家代码转换为 emoji 旗帜"""
    if not country_code or country_code == "null":
        return "🌐"
    
    # 将国家代码转换为区域指示符号
    # A=127462, 所以 'US' -> 🇺🇸
    try:
        first = ord(country_code[0].upper()) - ord('A') + 127462
        second = ord(country_code[1].upper()) - ord('A') + 127462
        return chr(first) + chr(second)
    except:
        return "🌐"


def signal_handler(sig, frame):
    """处理 Ctrl+C 信号"""
    print()
    print("---")
    loss_count = total_count - success_count
    loss_rate = 0.0
    if total_count > 0:
        loss_rate = loss_count / total_count * 100
    print(f"{total_count} requests transmitted, {success_count} received, {loss_count} failed, {loss_rate:.1f}% packet loss")
    sys.exit(0)


def test_http_proxy_continuous():
    """持续测试 HTTP 代理"""
    global total_count, success_count
    
    proxy_url = f"http://{PROXY_HOST}:{PROXY_PORT}"
    proxies = {
        "http": proxy_url,
        "https": proxy_url,
    }
    
    print(f"PROXY {PROXY_HOST}:{PROXY_PORT} ({TEST_URL}): continuous mode")
    print()
    
    # 注册信号处理
    signal.signal(signal.SIGINT, signal_handler)
    
    while True:
        total_count += 1
        
        try:
            start_time = time.time()
            response = requests.get(
                TEST_URL,
                proxies=proxies,
                timeout=15,
            )
            elapsed = int((time.time() - start_time) * 1000)
            
            if response.status_code == 200:
                data = response.json()
                exit_ip = data.get("query", "Unknown")
                country_code = data.get("countryCode", "")
                flag = country_to_emoji(country_code)
                print(f"proxy from {flag} {exit_ip}: seq={total_count} time={elapsed}ms")
                success_count += 1
            else:
                print(f"proxy #{total_count}: request failed (HTTP {response.status_code})")
                
        except RequestException as e:
            error_msg = str(e).split(':')[0]
            print(f"proxy #{total_count}: {error_msg}")
        
        time.sleep(DELAY_SECONDS)


if __name__ == "__main__":
    test_http_proxy_continuous()
