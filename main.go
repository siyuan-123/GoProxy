package main

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"proxy-pool/checker"
	"proxy-pool/config"
	"proxy-pool/fetcher"
	"proxy-pool/logger"
	"proxy-pool/proxy"
	"proxy-pool/storage"
	"proxy-pool/validator"
	"proxy-pool/webui"
)

var fetchRunning atomic.Bool
var fetchMu sync.Mutex

func main() {
	// 初始化日志收集器
	logger.Init()

	// 加载配置（优先读取 config.json）
	cfg := config.Load()

	// 初始化存储
	store, err := storage.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	defer store.Close()

	// 初始化模块
	fetch := fetcher.New(cfg.HTTPSourceURL, cfg.SOCKS5SourceURL)
	check := checker.New(store, validator.New(cfg.ValidateConcurrency, cfg.ValidateTimeout, cfg.ValidateURL), cfg)
	server := proxy.New(store, cfg)

	// 配置变更通知 channel
	configChanged := make(chan struct{}, 1)

	// 启动 WebUI
	ui := webui.New(store, cfg, func() {
		go func() {
			if err := fetchAndValidate(fetch, store); err != nil {
				log.Printf("[webui] fetch error: %v", err)
			}
		}()
	}, configChanged)
	ui.Start()

	// 后台启动：首次抓取验证
	go func() {
		log.Println("[main] fetching proxies on startup...")
		if err := fetchAndValidate(fetch, store); err != nil {
			log.Printf("[main] initial fetch error: %v", err)
		}
	}()

	// 启动动态定时抓取
	go startFetchLoop(fetch, store, configChanged)

	// 启动定时健康检查
	check.Start()

	// 启动代理服务（阻塞）
	if err := server.Start(); err != nil {
		log.Fatalf("proxy server: %v", err)
	}
}

func fetchAndValidate(fetch *fetcher.Fetcher, store *storage.Storage) error {
	// 防止并发执行
	if !fetchRunning.CompareAndSwap(false, true) {
		log.Println("[main] fetch already running, skipping")
		return nil
	}
	defer fetchRunning.Store(false)

	proxies, err := fetch.Fetch()
	if err != nil {
		return err
	}
	log.Printf("[main] validating %d proxies (streaming)...", len(proxies))

	// 每次用最新配置创建 validator
	cfg := config.Get()
	validate := validator.New(cfg.ValidateConcurrency, cfg.ValidateTimeout, cfg.ValidateURL)

	var valid, total int
	for r := range validate.ValidateStream(proxies) {
		total++
		if total%1000 == 0 {
			log.Printf("[main] checked=%d/%d valid=%d", total, len(proxies), valid)
		}
		if r.Valid {
			if valid == 0 {
				log.Printf("[main] first valid proxy: %s (%s) latency=%v", r.Proxy.Address, r.Proxy.Protocol, r.Latency)
			}
			valid++
			if err := store.AddProxy(r.Proxy.Address, r.Proxy.Protocol); err != nil {
				log.Printf("[main] addProxy error: %v", err)
			}
			if valid%10 == 0 {
				log.Printf("[main] progress: valid=%d checked=%d/%d", valid, total, len(proxies))
			}
		}
	}

	count, _ := store.Count()
	log.Printf("[main] validated: valid=%d/%d, pool size=%d", valid, len(proxies), count)
	return nil
}

func startFetchLoop(fetch *fetcher.Fetcher, store *storage.Storage, configChanged <-chan struct{}) {
	cfg := config.Get()
	ticker := time.NewTicker(time.Duration(cfg.FetchInterval) * time.Minute)
	log.Printf("[main] fetch loop started, interval: %d min", cfg.FetchInterval)
	for {
		select {
		case <-ticker.C:
			log.Println("[main] scheduled fetch started...")
			if err := fetchAndValidate(fetch, store); err != nil {
				log.Printf("[main] scheduled fetch error: %v", err)
			}
		case <-configChanged:
			newCfg := config.Get()
			ticker.Reset(time.Duration(newCfg.FetchInterval) * time.Minute)
			log.Printf("[main] fetch interval updated to %d min", newCfg.FetchInterval)
		}
	}
}
