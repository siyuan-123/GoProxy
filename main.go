package main

import (
	"log"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"goproxy/checker"
	"goproxy/config"
	"goproxy/custom"
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

// 候选缓存：保存一轮抓取的全部候选，分批验证，确保所有候选都被处理
type candidateCache struct {
	httpCandidates   []storage.Proxy
	socks5Candidates []storage.Proxy
	httpOffset       int
	socks5Offset     int
}

// exhausted 判断缓存是否全部处理完毕（或为空）
func (c *candidateCache) exhausted() bool {
	return c.httpOffset >= len(c.httpCandidates) && c.socks5Offset >= len(c.socks5Candidates)
}

// reset 清空缓存，准备下一轮抓取
func (c *candidateCache) reset() {
	c.httpCandidates = nil
	c.socks5Candidates = nil
	c.httpOffset = 0
	c.socks5Offset = 0
}

var candCache candidateCache

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
	
	// 清理无效代理（免费代理删除，订阅代理禁用）
	totalDeleted := 0
	if len(cfg.AllowedCountries) > 0 {
		if deleted, err := store.DeleteNotAllowedCountries(cfg.AllowedCountries); err == nil && deleted > 0 {
			log.Printf("[main] 🧹 已清理 %d 个非白名单免费代理 (允许: %v)", deleted, cfg.AllowedCountries)
			totalDeleted += int(deleted)
		}
		if disabled, err := store.DisableNotAllowedCountries(cfg.AllowedCountries); err == nil && disabled > 0 {
			log.Printf("[main] 🔒 已禁用 %d 个非白名单订阅代理", disabled)
		}
	} else if len(cfg.BlockedCountries) > 0 {
		if deleted, err := store.DeleteBlockedCountries(cfg.BlockedCountries); err == nil && deleted > 0 {
			log.Printf("[main] 🧹 已清理 %d 个屏蔽国家免费代理 (屏蔽: %v)", deleted, cfg.BlockedCountries)
			totalDeleted += int(deleted)
		}
		if disabled, err := store.DisableBlockedCountries(cfg.BlockedCountries); err == nil && disabled > 0 {
			log.Printf("[main] 🔒 已禁用 %d 个屏蔽国家订阅代理", disabled)
		}
	}
	if deleted, err := store.DeleteWithoutExitInfo(); err == nil && deleted > 0 {
		log.Printf("[main] 🧹 已清理 %d 个无出口信息的代理", deleted)
		totalDeleted += int(deleted)
	}
	
	// 创建 HTTP 代理服务器：随机轮换 + 最低延迟（传入 poolMgr 用于 warm 池即时提升）
	randomServer := proxy.New(store, cfg, "random", cfg.ProxyPort, poolMgr)
	stableServer := proxy.New(store, cfg, "lowest-latency", cfg.StableProxyPort, poolMgr)
	
	// 创建 SOCKS5 代理服务器：随机轮换 + 最低延迟
	socks5RandomServer := proxy.NewSOCKS5(store, cfg, "random", cfg.SOCKS5Port, poolMgr)
	socks5StableServer := proxy.NewSOCKS5(store, cfg, "lowest-latency", cfg.StableSOCKS5Port, poolMgr)

	// 初始化订阅管理器
	customMgr := custom.NewManager(store, validate, cfg)

	// 配置变更通知 channel
	configChanged := make(chan struct{}, 1)

	// 启动 WebUI（传递池子管理器和订阅管理器）
	ui := webui.New(store, cfg, poolMgr, customMgr, func() {
		go smartFetchAndFill(fetch, validate, store, poolMgr, true)
	}, configChanged)
	ui.Start()

	// 首次智能填充（清理后立即触发）
	go func() {
		if totalDeleted > 0 {
			log.Printf("[main] 🚀 清理后立即启动补充填充...")
		} else {
			log.Println("[main] 🚀 启动初始化填充...")
		}
		smartFetchAndFill(fetch, validate, store, poolMgr, false)
	}()

	// 启动 warm→active 自动提升协程
	go startWarmPromoter(poolMgr)

	// 启动状态监控协程
	go startStatusMonitor(poolMgr, fetch, validate, store)

	// 启动 5 分钟主动拉取定时器（快速源）
	go startProactiveFetch(fetch, validate, poolMgr)

	// 启动健康检查器
	healthChecker.StartBackground()

	// 启动优化轮换器
	opt.StartBackground()

	// 启动订阅管理器
	go customMgr.Start()

	// 监听配置变更
	go watchConfigChanges(configChanged, poolMgr)

	// 启动 HTTP 稳定代理服务（最低延迟模式）
	go func() {
		if err := stableServer.Start(); err != nil {
			log.Fatalf("stable http proxy server: %v", err)
		}
	}()

	// 启动 SOCKS5 稳定代理服务（最低延迟模式）
	go func() {
		if err := socks5StableServer.Start(); err != nil {
			log.Fatalf("stable socks5 proxy server: %v", err)
		}
	}()

	// 启动 SOCKS5 随机代理服务
	go func() {
		if err := socks5RandomServer.Start(); err != nil {
			log.Fatalf("random socks5 proxy server: %v", err)
		}
	}()

	// 启动 HTTP 随机代理服务（阻塞）
	if err := randomServer.Start(); err != nil {
		log.Fatalf("random http proxy server: %v", err)
	}
}

// smartFetchAndFill 智能抓取和填充。forceRefetch=true 时清空候选缓存强制重新抓取。
func smartFetchAndFill(fetch *fetcher.Fetcher, validate *validator.Validator, store *storage.Storage, poolMgr *pool.Manager, forceRefetch bool) {
	// 防止并发执行
	if !fetchRunning.CompareAndSwap(false, true) {
		log.Println("[main] 抓取已在运行，跳过")
		return
	}
	defer fetchRunning.Store(false)

	if forceRefetch && !candCache.exhausted() {
		log.Printf("[main] 强制重新抓取，丢弃缓存剩余（HTTP %d/%d, SOCKS5 %d/%d）",
			len(candCache.httpCandidates)-candCache.httpOffset, len(candCache.httpCandidates),
			len(candCache.socks5Candidates)-candCache.socks5Offset, len(candCache.socks5Candidates))
		candCache.reset()
	}

	// 获取池子状态
	status, err := poolMgr.GetStatus()
	if err != nil {
		log.Printf("[main] 获取池子状态失败: %v", err)
		return
	}

	log.Printf("[main] 📊 池子状态: %s | HTTP=%d/%d SOCKS5=%d/%d 总计=%d/%d | warm: HTTP=%d SOCKS5=%d",
		status.State, status.HTTP, status.HTTPSlots, status.SOCKS5, status.SOCKS5Slots,
		status.Total, config.Get().PoolMaxSize, status.WarmHTTP, status.WarmSOCKS5)

	cfg := config.Get()
	maxPerProto := cfg.MaxCandidatesPerProtocol
	if maxPerProto <= 0 {
		maxPerProto = 3000
	}

	if candCache.exhausted() {
		// 缓存为空或已用完，判断是否需要重新抓取
		needFetch, mode, preferredProtocol := poolMgr.NeedsFetch(status)
		if !needFetch {
			log.Println("[main] 池子健康，无需抓取")
			return
		}

		log.Printf("[main] 🔍 智能抓取: 模式=%s 协议偏好=%s", mode, preferredProtocol)

		candidates, err := fetch.FetchSmart(mode, preferredProtocol)
		if err != nil {
			log.Printf("[main] 抓取失败: %v", err)
			return
		}

		// 填充缓存，打乱顺序确保各批次来源多样
		candCache.reset()
		for _, c := range candidates {
			if c.Protocol == "http" {
				candCache.httpCandidates = append(candCache.httpCandidates, c)
			} else {
				candCache.socks5Candidates = append(candCache.socks5Candidates, c)
			}
		}
		rand.Shuffle(len(candCache.httpCandidates), func(i, j int) {
			candCache.httpCandidates[i], candCache.httpCandidates[j] = candCache.httpCandidates[j], candCache.httpCandidates[i]
		})
		rand.Shuffle(len(candCache.socks5Candidates), func(i, j int) {
			candCache.socks5Candidates[i], candCache.socks5Candidates[j] = candCache.socks5Candidates[j], candCache.socks5Candidates[i]
		})

		log.Printf("[main] 📦 缓存新一轮候选: HTTP=%d SOCKS5=%d，每批上限 %d",
			len(candCache.httpCandidates), len(candCache.socks5Candidates), maxPerProto)
	} else {
		// 缓存有剩余候选，检查池子是否还需要
		needFetch, _, _ := poolMgr.NeedsFetch(status)
		if !needFetch {
			log.Printf("[main] 池子健康，暂停处理缓存（HTTP %d/%d, SOCKS5 %d/%d）",
				candCache.httpOffset, len(candCache.httpCandidates),
				candCache.socks5Offset, len(candCache.socks5Candidates))
			return
		}
	}

	// 从缓存取下一批候选
	httpEnd := candCache.httpOffset + maxPerProto
	if httpEnd > len(candCache.httpCandidates) {
		httpEnd = len(candCache.httpCandidates)
	}
	httpBatch := candCache.httpCandidates[candCache.httpOffset:httpEnd]
	candCache.httpOffset = httpEnd

	socks5End := candCache.socks5Offset + maxPerProto
	if socks5End > len(candCache.socks5Candidates) {
		socks5End = len(candCache.socks5Candidates)
	}
	socks5Batch := candCache.socks5Candidates[candCache.socks5Offset:socks5End]
	candCache.socks5Offset = socks5End

	log.Printf("[main] 本批验证: HTTP=%d SOCKS5=%d | 缓存进度: HTTP %d/%d SOCKS5 %d/%d",
		len(httpBatch), len(socks5Batch),
		candCache.httpOffset, len(candCache.httpCandidates),
		candCache.socks5Offset, len(candCache.socks5Candidates))

	// 共享计数器
	var addedCount atomic.Int32
	var validCount atomic.Int32
	var rejectedNoExit atomic.Int32
	var rejectedLatency atomic.Int32
	var rejectedGeo atomic.Int32
	var rejectedFull atomic.Int32

	// 入池处理函数（两个协程共用）
	processResult := func(result validator.Result) {
		if !result.Valid {
			return
		}

		validCount.Add(1)
		latencyMs := int(result.Latency.Milliseconds())

		cfg := config.Get()
		maxLatency := cfg.GetLatencyThreshold(status.State)

		if result.ExitIP == "" || result.ExitLocation == "" {
			rejectedNoExit.Add(1)
			return
		}

		if latencyMs > maxLatency {
			rejectedLatency.Add(1)
			return
		}

		proxyToAdd := storage.Proxy{
			Address:      result.Proxy.Address,
			Protocol:     result.Proxy.Protocol,
			ExitIP:       result.ExitIP,
			ExitLocation: result.ExitLocation,
			Latency:      latencyMs,
		}

		httpsMs := int(result.HTTPSLatency.Milliseconds())
		if added, reason := poolMgr.TryAddProxy(proxyToAdd, httpsMs); added {
			addedCount.Add(1)
		} else if reason == "slots_full" {
			rejectedFull.Add(1)
		} else if len(result.ExitLocation) >= 2 {
			countryCode := result.ExitLocation[:2]
			for _, blocked := range cfg.BlockedCountries {
				if countryCode == blocked {
					rejectedGeo.Add(1)
					break
				}
			}
		}
	}

	// 池子是否已满的检查函数
	poolFilled := func() bool {
		currentStatus, _ := poolMgr.GetStatus()
		return !poolMgr.NeedsFetchQuick(currentStatus)
	}

	var wg sync.WaitGroup

	// SOCKS5 协程：验证快，优先填充
	if len(socks5Batch) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			count := 0
			for result := range validate.ValidateStream(socks5Batch) {
				processResult(result)
				count++
				if count%20 == 0 && poolFilled() {
					log.Println("[main] ✅ SOCKS5 验证中检测到池子已满，停止")
					break
				}
			}
			log.Printf("[main] SOCKS5 验证完成，处理 %d 个", count)
		}()
	}

	// HTTP 协程：有额外 HTTPS 检测，较慢
	if len(httpBatch) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			count := 0
			for result := range validate.ValidateStream(httpBatch) {
				processResult(result)
				count++
				if count%20 == 0 && poolFilled() {
					log.Println("[main] ✅ HTTP 验证中检测到池子已满，停止")
					break
				}
			}
			log.Printf("[main] HTTP 验证完成，处理 %d 个", count)
		}()
	}

	wg.Wait()

	// 最终状态
	finalStatus, _ := poolMgr.GetStatus()
	log.Printf("[main] 本批完成: 验证%d 通过%d 入池%d | 拒绝[无出口:%d 延迟:%d 地理:%d 满:%d] | 最终: %s HTTP=%d SOCKS5=%d | warm: HTTP=%d SOCKS5=%d",
		len(httpBatch)+len(socks5Batch), validCount.Load(), addedCount.Load(),
		rejectedNoExit.Load(), rejectedLatency.Load(), rejectedGeo.Load(), rejectedFull.Load(),
		finalStatus.State, finalStatus.HTTP, finalStatus.SOCKS5, finalStatus.WarmHTTP, finalStatus.WarmSOCKS5)

	// 检查本轮是否全部处理完毕
	if candCache.exhausted() {
		log.Printf("[main] ✅ 本轮全部候选处理完毕（HTTP=%d SOCKS5=%d），下次将重新抓取",
			len(candCache.httpCandidates), len(candCache.socks5Candidates))
		candCache.reset()
	} else {
		log.Printf("[main] 📋 缓存剩余: HTTP %d/%d SOCKS5 %d/%d，下次补池继续处理",
			len(candCache.httpCandidates)-candCache.httpOffset, len(candCache.httpCandidates),
			len(candCache.socks5Candidates)-candCache.socks5Offset, len(candCache.socks5Candidates))
	}
}

// startWarmPromoter 定期检查 active 池是否掉量，自动从 warm 池提升
// 健康时 10s 一次，critical/emergency 时加速到 2s
func startWarmPromoter(poolMgr *pool.Manager) {
	log.Println("[warm] warm→active 自动提升器已启动（正常10s/紧急2s）")

	normalInterval := 10 * time.Second
	urgentInterval := 2 * time.Second
	ticker := time.NewTicker(normalInterval)
	currentInterval := normalInterval

	for range ticker.C {
		promoted := poolMgr.PromoteIfNeeded()
		if promoted > 0 {
			log.Printf("[warm] 本轮从 warm 池提升 %d 个代理到 active", promoted)
		}

		// 根据池子状态动态调整检查频率
		status, err := poolMgr.GetStatus()
		if err == nil {
			var targetInterval time.Duration
			if status.State == "emergency" || status.State == "critical" {
				targetInterval = urgentInterval
			} else {
				targetInterval = normalInterval
			}
			if targetInterval != currentInterval {
				ticker.Reset(targetInterval)
				currentInterval = targetInterval
			}
		}
	}
}

// startProactiveFetch 每 5 分钟主动从快速源拉取新代理，替换池中差代理
func startProactiveFetch(fetch *fetcher.Fetcher, validate *validator.Validator, poolMgr *pool.Manager) {
	ticker := time.NewTicker(5 * time.Minute)
	log.Println("[proactive] 主动拉取器已启动（每5分钟从快速源拉取）")

	for range ticker.C {
		proactiveFetchAndOptimize(fetch, validate, poolMgr)
	}
}

// proactiveFetchAndOptimize 主动拉取快速源并尝试替换差代理
func proactiveFetchAndOptimize(fetch *fetcher.Fetcher, validate *validator.Validator, poolMgr *pool.Manager) {
	if !fetchRunning.CompareAndSwap(false, true) {
		log.Println("[proactive] 有抓取任务在运行，跳过本轮主动拉取")
		return
	}
	defer fetchRunning.Store(false)

	candidates, err := fetch.FetchFast()
	if err != nil {
		log.Printf("[proactive] 快速源拉取失败: %v", err)
		return
	}

	if len(candidates) == 0 {
		log.Println("[proactive] 无新代理（均已去重），跳过")
		return
	}

	log.Printf("[proactive] 拉取到 %d 个新候选代理，开始验证...", len(candidates))

	cfg := config.Get()
	status, _ := poolMgr.GetStatus()

	var addedCount, validCount, replacedCount int32

	for result := range validate.ValidateStream(candidates) {
		if !result.Valid {
			continue
		}

		validCount++

		latencyMs := int(result.Latency.Milliseconds())
		maxLatency := cfg.GetLatencyThreshold(status.State)

		if result.ExitIP == "" || result.ExitLocation == "" {
			continue
		}
		if latencyMs > maxLatency {
			continue
		}

		proxyToAdd := storage.Proxy{
			Address:      result.Proxy.Address,
			Protocol:     result.Proxy.Protocol,
			ExitIP:       result.ExitIP,
			ExitLocation: result.ExitLocation,
			Latency:      latencyMs,
		}

		httpsMs := int(result.HTTPSLatency.Milliseconds())
		if added, reason := poolMgr.TryAddProxy(proxyToAdd, httpsMs); added {
			addedCount++
			if reason == "replaced" {
				replacedCount++
			}
		}
	}

	if validCount > 0 || addedCount > 0 {
		log.Printf("[proactive] 主动拉取完成: 验证通过 %d 入池 %d 替换 %d",
			validCount, addedCount, replacedCount)
	}
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
			go smartFetchAndFill(fetch, validate, store, poolMgr, false)
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
