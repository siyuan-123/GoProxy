package main

import (
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"goproxy/checker"
	"goproxy/config"
	"goproxy/fetcher"
	"goproxy/logger"
	"goproxy/optimizer"
	"goproxy/pool"
	"goproxy/proxy"
	"goproxy/storage"
	"goproxy/validator"
	"goproxy/webui"
)

var fetchRunning atomic.Bool
var fetchMu sync.Mutex

func main() {
	// 初始化日志收集器
	logger.Init()

	// 加载配置
	cfg := config.Load()

	// 提示密码信息
	if os.Getenv("WEBUI_PASSWORD") == "" {
		log.Printf("[main] WebUI 使用默认密码: %s（可通过环境变量 WEBUI_PASSWORD 自定义）", config.DefaultPassword)
	} else {
		log.Println("[main] WebUI 密码已通过环境变量 WEBUI_PASSWORD 设置")
	}

	log.Printf("[main] 🎯 智能代理池配置: 容量=%d HTTP=%.0f%% SOCKS5=%.0f%% 延迟标准=%dms",
		cfg.PoolMaxSize, cfg.PoolHTTPRatio*100, (1-cfg.PoolHTTPRatio)*100, cfg.MaxLatencyMs)

	// 初始化存储
	store, err := storage.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	defer store.Close()

	// 初始化限流器
	fetcher.InitIPQueryLimiter(cfg.IPQueryRateLimit)

	// 初始化核心模块
	sourceMgr := fetcher.NewSourceManager(store.GetDB())
	fetch := fetcher.New(cfg.HTTPSourceURL, cfg.SOCKS5SourceURL, sourceMgr)
	validate := validator.New(cfg.ValidateConcurrency, cfg.ValidateTimeout, cfg.ValidateURL)
	poolMgr := pool.NewManager(store, cfg)
	healthChecker := checker.NewHealthChecker(store, validate, cfg, poolMgr)
	opt := optimizer.NewOptimizer(store, fetch, validate, poolMgr, cfg)
	
	// 清理无效代理
	totalDeleted := 0
	if deleted, err := store.DeleteChinaMainland(); err == nil && deleted > 0 {
		log.Printf("[main] 🧹 已清理 %d 个中国大陆出口代理", deleted)
		totalDeleted += int(deleted)
	}
	if deleted, err := store.DeleteWithoutExitInfo(); err == nil && deleted > 0 {
		log.Printf("[main] 🧹 已清理 %d 个无出口信息的代理", deleted)
		totalDeleted += int(deleted)
	}
	
	// 创建两个代理服务器：随机轮换 + 最低延迟
	randomServer := proxy.New(store, cfg, "random", cfg.ProxyPort)
	stableServer := proxy.New(store, cfg, "lowest-latency", cfg.StableProxyPort)

	// 配置变更通知 channel
	configChanged := make(chan struct{}, 1)

	// 启动 WebUI（传递池子管理器）
	ui := webui.New(store, cfg, poolMgr, func() {
		go smartFetchAndFill(fetch, validate, store, poolMgr)
	}, configChanged)
	ui.Start()

	// 首次智能填充（清理后立即触发）
	go func() {
		if totalDeleted > 0 {
			log.Printf("[main] 🚀 清理后立即启动补充填充...")
		} else {
			log.Println("[main] 🚀 启动初始化填充...")
		}
		smartFetchAndFill(fetch, validate, store, poolMgr)
	}()

	// 启动状态监控协程
	go startStatusMonitor(poolMgr, fetch, validate, store)

	// 启动健康检查器
	healthChecker.StartBackground()

	// 启动优化轮换器
	opt.StartBackground()

	// 监听配置变更
	go watchConfigChanges(configChanged, poolMgr)

	// 启动稳定代理服务（最低延迟模式）
	go func() {
		if err := stableServer.Start(); err != nil {
			log.Fatalf("stable proxy server: %v", err)
		}
	}()

	// 启动随机代理服务（阻塞）
	if err := randomServer.Start(); err != nil {
		log.Fatalf("random proxy server: %v", err)
	}
}

// smartFetchAndFill 智能抓取和填充
func smartFetchAndFill(fetch *fetcher.Fetcher, validate *validator.Validator, store *storage.Storage, poolMgr *pool.Manager) {
	// 防止并发执行
	if !fetchRunning.CompareAndSwap(false, true) {
		log.Println("[main] 抓取已在运行，跳过")
		return
	}
	defer fetchRunning.Store(false)

	// 获取池子状态
	status, err := poolMgr.GetStatus()
	if err != nil {
		log.Printf("[main] 获取池子状态失败: %v", err)
		return
	}

	log.Printf("[main] 📊 池子状态: %s | HTTP=%d/%d SOCKS5=%d/%d 总计=%d/%d",
		status.State, status.HTTP, status.HTTPSlots, status.SOCKS5, status.SOCKS5Slots,
		status.Total, config.Get().PoolMaxSize)

	// 判断是否需要抓取
	needFetch, mode, preferredProtocol := poolMgr.NeedsFetch(status)
	if !needFetch {
		log.Println("[main] 池子健康，无需抓取")
		return
	}

	log.Printf("[main] 🔍 智能抓取: 模式=%s 协议偏好=%s", mode, preferredProtocol)

	// 智能抓取
	candidates, err := fetch.FetchSmart(mode, preferredProtocol)
	if err != nil {
		log.Printf("[main] 抓取失败: %v", err)
		return
	}

	log.Printf("[main] 抓取到 %d 个候选代理，开始严格验证...", len(candidates))

	// 严格验证并尝试入池
	addedCount := 0
	validCount := 0

	for result := range validate.ValidateStream(candidates) {
		if !result.Valid {
			continue
		}

		validCount++
		latencyMs := int(result.Latency.Milliseconds())

		// 根据池子状态动态调整延迟标准
		cfg := config.Get()
		maxLatency := cfg.GetLatencyThreshold(status.State)

		// 必须满足：有出口IP、有位置、延迟达标
		if result.ExitIP == "" || result.ExitLocation == "" || latencyMs > maxLatency {
			continue
		}

		// 尝试加入池子
		proxyToAdd := storage.Proxy{
			Address:      result.Proxy.Address,
			Protocol:     result.Proxy.Protocol,
			ExitIP:       result.ExitIP,
			ExitLocation: result.ExitLocation,
			Latency:      latencyMs,
		}

		if added, _ := poolMgr.TryAddProxy(proxyToAdd); added {
			addedCount++
		}

		// 如果是紧急模式且已达到最小要求，停止验证
		if mode == "emergency" && status.HTTP >= cfg.PoolMinPerProtocol && status.SOCKS5 >= cfg.PoolMinPerProtocol {
			log.Println("[main] 🎉 紧急模式：达到最小要求，停止验证")
			break
		}

		// 动态检查是否已经填满
		if addedCount > 0 && addedCount%20 == 0 {
			currentStatus, _ := poolMgr.GetStatus()
			if !poolMgr.NeedsFetchQuick(currentStatus) {
				log.Println("[main] ✅ 池子已填满，停止验证")
				break
			}
		}
	}

	// 最终状态
	finalStatus, _ := poolMgr.GetStatus()
	log.Printf("[main] 填充完成: 验证%d 通过%d 入池%d | 最终状态: %s HTTP=%d SOCKS5=%d",
		len(candidates), validCount, addedCount,
		finalStatus.State, finalStatus.HTTP, finalStatus.SOCKS5)
}

// startStatusMonitor 状态监控协程
func startStatusMonitor(poolMgr *pool.Manager, fetch *fetcher.Fetcher, validate *validator.Validator, store *storage.Storage) {
	ticker := time.NewTicker(30 * time.Second)
	log.Println("[monitor] 📡 状态监控器已启动（每30秒检查）")

	for range ticker.C {
		status, err := poolMgr.GetStatus()
		if err != nil {
			continue
		}

		// 每分钟检查池子状态
		needFetch, mode, preferredProtocol := poolMgr.NeedsFetch(status)
		if needFetch {
			log.Printf("[monitor] ⚠️  检测到池子需求: 状态=%s 模式=%s 协议=%s",
				status.State, mode, preferredProtocol)
			// 触发智能填充
			go smartFetchAndFill(fetch, validate, store, poolMgr)
		}
	}
}

// watchConfigChanges 监听配置变更
func watchConfigChanges(configChanged <-chan struct{}, poolMgr *pool.Manager) {
	var oldSize int
	var oldRatio float64

	cfg := config.Get()
	oldSize = cfg.PoolMaxSize
	oldRatio = cfg.PoolHTTPRatio

	for range configChanged {
		newCfg := config.Get()
		if newCfg.PoolMaxSize != oldSize || newCfg.PoolHTTPRatio != oldRatio {
			log.Printf("[config] 🔧 配置变更检测: 容量 %d→%d 比例 %.2f→%.2f",
				oldSize, newCfg.PoolMaxSize, oldRatio, newCfg.PoolHTTPRatio)
			poolMgr.AdjustForConfigChange(oldSize, oldRatio)
			oldSize = newCfg.PoolMaxSize
			oldRatio = newCfg.PoolHTTPRatio
		}
	}
}
