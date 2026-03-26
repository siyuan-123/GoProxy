package config

import (
	"encoding/json"
	"os"
	"sync"
)

func dataDir() string {
	if d := os.Getenv("DATA_DIR"); d != "" {
		os.MkdirAll(d, 0755)
		return d + "/"
	}
	return ""
}

func ConfigFile() string { return dataDir() + "config.json" }

type Config struct {
	// WebUI 端口
	WebUIPort string

	// WebUI 密码 SHA256 哈希
	WebUIPasswordHash string

	// 代理池本地监听端口
	ProxyPort string

	// SQLite 数据库路径
	DBPath string

	// 验证并发数
	ValidateConcurrency int

	// 验证超时（秒）
	ValidateTimeout int

	// 验证目标 URL
	ValidateURL string

	// 最大响应时间（毫秒），超过则丢弃
	MaxResponseMs int

	// 代理失败次数阈值，超过后删除
	MaxFailCount int

	// 自动重试次数
	MaxRetry int

	// 定时抓取间隔（分钟）
	FetchInterval int

	// 定时健康检查间隔（分钟）
	CheckInterval int

	// 代理来源 URL
	HTTPSourceURL   string
	SOCKS5SourceURL string
}

var (
	globalCfg *Config
	cfgMu     sync.RWMutex
)

func DefaultConfig() *Config {
	return &Config{
		WebUIPort:           ":7778",
		WebUIPasswordHash:   "64c2de42ff93286f5c7108867ffe3167a24f4c1abee648dea7bc7fa1d11e2b21",
		ProxyPort:           ":7777",
		DBPath:              dataDir() + "proxy.db",
		ValidateConcurrency: 300,
		ValidateTimeout:     3,
		ValidateURL:         "https://cursor.com/api/auth/me",
		MaxResponseMs:       2500,
		MaxFailCount:        3,
		MaxRetry:            3,
		FetchInterval:       30,
		CheckInterval:       10,
		HTTPSourceURL:       "https://cdn.jsdelivr.net/gh/databay-labs/free-proxy-list/http.txt",
		SOCKS5SourceURL:     "https://cdn.jsdelivr.net/gh/databay-labs/free-proxy-list/socks5.txt",
	}
}

// Load 从文件加载配置，文件不存在则用默认值
func Load() *Config {
	cfg := DefaultConfig()
	data, err := os.ReadFile(ConfigFile())
	if err == nil {
		// 只覆盖可调整的4个字段
		var saved savedConfig
		if json.Unmarshal(data, &saved) == nil {
			if saved.FetchInterval > 0 {
				cfg.FetchInterval = saved.FetchInterval
			}
			if saved.CheckInterval > 0 {
				cfg.CheckInterval = saved.CheckInterval
			}
			if saved.ValidateConcurrency > 0 {
				cfg.ValidateConcurrency = saved.ValidateConcurrency
			}
			if saved.ValidateTimeout > 0 {
				cfg.ValidateTimeout = saved.ValidateTimeout
			}
		}
	}
	cfgMu.Lock()
	globalCfg = cfg
	cfgMu.Unlock()
	return cfg
}

// Get 获取当前配置
func Get() *Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return globalCfg
}

// savedConfig 只持久化可调整的字段
type savedConfig struct {
	FetchInterval       int `json:"fetch_interval"`
	CheckInterval       int `json:"check_interval"`
	ValidateConcurrency int `json:"validate_concurrency"`
	ValidateTimeout     int `json:"validate_timeout"`
}

// Save 保存可调整字段到文件，并更新内存配置
func Save(fetchInterval, checkInterval, validateConcurrency, validateTimeout int) error {
	cfgMu.Lock()
	globalCfg.FetchInterval = fetchInterval
	globalCfg.CheckInterval = checkInterval
	globalCfg.ValidateConcurrency = validateConcurrency
	globalCfg.ValidateTimeout = validateTimeout
	cfgMu.Unlock()

	data, err := json.MarshalIndent(savedConfig{
		FetchInterval:       fetchInterval,
		CheckInterval:       checkInterval,
		ValidateConcurrency: validateConcurrency,
		ValidateTimeout:     validateTimeout,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigFile(), data, 0644)
}
