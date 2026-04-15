package checker

import (
	"log"
	"time"

	"goproxy/config"
	"goproxy/pool"
	"goproxy/storage"
	"goproxy/validator"
)

// HealthChecker 健康检查器
type HealthChecker struct {
	storage   *storage.Storage
	validator *validator.Validator
	cfg       *config.Config
	poolMgr   *pool.Manager
}

func NewHealthChecker(s *storage.Storage, v *validator.Validator, cfg *config.Config, pm *pool.Manager) *HealthChecker {
	return &HealthChecker{
		storage:   s,
		validator: v,
		cfg:       cfg,
		poolMgr:   pm,
	}
}

// RunOnce 执行一次健康检查
func (hc *HealthChecker) RunOnce() {
	start := time.Now()
	log.Println("[health] 开始健康检查...")

	// 全量检查：所有等级代理都参与，不分批
	proxies, err := hc.storage.GetBatchForHealthCheck(0, false)
	if err != nil {
		log.Printf("[health] 获取检查批次失败: %v", err)
		return
	}

	if len(proxies) == 0 {
		log.Println("[health] 无需检查的代理")
		return
	}

	log.Printf("[health] 检查 %d 个代理", len(proxies))

	// 防雪崩：单次健康检查最多移除 50% 的被检代理
	maxRemove := len(proxies) / 2
	if maxRemove < 3 {
		maxRemove = 3
	}

	validCount := 0
	removeCount := 0
	degradeCount := 0
	updateCount := 0

	for result := range hc.validator.ValidateStream(proxies) {
		if result.Valid {
			validCount++
			latencyMs := int(result.Latency.Milliseconds())
			if err := hc.storage.UpdateExitInfo(result.Proxy.Address, result.ExitIP, result.ExitLocation, latencyMs); err == nil {
				updateCount++
			}
			hc.storage.ResetFail(result.Proxy.Address)
			hc.storage.ResetConsecutiveFails(result.Proxy.Address)
			if result.HTTPSLatency > 0 {
				httpsMs := int(result.HTTPSLatency.Milliseconds())
				hc.storage.UpdateKiroValidation(result.Proxy.Address, httpsMs)
			}
		} else {
			hc.storage.IncrementFailCount(result.Proxy.Address)
			consecutiveCount, _ := hc.storage.IncrementConsecutiveFails(result.Proxy.Address)
			shouldRemove := result.Proxy.FailCount+1 >= 3 || consecutiveCount >= hc.cfg.ConsecutiveFailThreshold
			if shouldRemove {
				if removeCount >= maxRemove {
					// 达到移除上限，只标记 degraded 不删除，等下一轮再处理
					hc.storage.MarkDegraded(result.Proxy.Address)
					degradeCount++
					continue
				}
				if result.Proxy.Source == "custom" {
					hc.storage.DisableProxy(result.Proxy.Address)
				} else {
					hc.storage.Delete(result.Proxy.Address)
				}
				removeCount++
			} else {
				hc.storage.MarkDegraded(result.Proxy.Address)
				degradeCount++
			}
		}
	}

	elapsed := time.Since(start)
	log.Printf("[health] 完成: 验证%d 有效%d 更新%d 移除%d 降级%d 耗时%v",
		len(proxies), validCount, updateCount, removeCount, degradeCount, elapsed)
}

// RunQuickCheck 快速连通性检测：只测 HTTP 204，不做 HTTPS/IP 探测
func (hc *HealthChecker) RunQuickCheck() {
	start := time.Now()

	proxies, err := hc.storage.GetBatchForHealthCheck(0, false)
	if err != nil || len(proxies) == 0 {
		return
	}

	aliveCount := 0
	removeCount := 0

	for result := range hc.validator.QuickCheckStream(proxies) {
		if result.Alive {
			aliveCount++
			// 不重置 consecutive_fails，避免干扰完整检查的 HTTPS 失败追踪
		} else {
			consecutiveCount, _ := hc.storage.IncrementConsecutiveFails(result.Proxy.Address)
			if consecutiveCount >= hc.cfg.ConsecutiveFailThreshold {
				if result.Proxy.Source == "custom" {
					hc.storage.DisableProxy(result.Proxy.Address)
				} else {
					hc.storage.Delete(result.Proxy.Address)
				}
				removeCount++
			}
		}
	}

	elapsed := time.Since(start)
	if removeCount > 0 {
		log.Printf("[quick-check] 检测%d 存活%d 移除%d 耗时%v", len(proxies), aliveCount, removeCount, elapsed)
	}
}

// RunWarmCheck 检查 warm 池代理的连通性，失败的直接删除
func (hc *HealthChecker) RunWarmCheck() {
	proxies, err := hc.storage.GetWarmProxies(0)
	if err != nil || len(proxies) == 0 {
		return
	}

	start := time.Now()
	aliveCount := 0
	removeCount := 0

	for result := range hc.validator.ValidateStream(proxies) {
		if result.Valid {
			aliveCount++
			latencyMs := int(result.Latency.Milliseconds())
			hc.storage.UpdateExitInfo(result.Proxy.Address, result.ExitIP, result.ExitLocation, latencyMs)
			hc.storage.ResetFail(result.Proxy.Address)
			hc.storage.ResetConsecutiveFails(result.Proxy.Address)
			if result.HTTPSLatency > 0 {
				httpsMs := int(result.HTTPSLatency.Milliseconds())
				hc.storage.UpdateKiroValidation(result.Proxy.Address, httpsMs)
			}
		} else {
			// warm 池代理失败 2 次直接删除，不做降级
			hc.storage.IncrementFailCount(result.Proxy.Address)
			count, _ := hc.storage.IncrementConsecutiveFails(result.Proxy.Address)
			if result.Proxy.FailCount+1 >= 2 || count >= 2 {
				if result.Proxy.Source == "custom" {
					hc.storage.DisableProxy(result.Proxy.Address)
				} else {
					hc.storage.Delete(result.Proxy.Address)
				}
				removeCount++
			}
		}
	}

	elapsed := time.Since(start)
	if removeCount > 0 || aliveCount > 0 {
		log.Printf("[health-warm] warm 池检查完成: %d 个存活 %d 个移除 耗时%v", aliveCount, removeCount, elapsed)
	}
}

// StartBackground 后台定时健康检查
func (hc *HealthChecker) StartBackground() {
	// 完整检查：每 HealthCheckInterval 分钟，含 Kiro HTTPS 探测
	fullTicker := time.NewTicker(time.Duration(hc.cfg.HealthCheckInterval) * time.Minute)
	go func() {
		for range fullTicker.C {
			hc.RunOnce()
		}
	}()

	// 快速连通性检测：每 1 分钟，只测基本连通
	quickTicker := time.NewTicker(1 * time.Minute)
	go func() {
		for range quickTicker.C {
			hc.RunQuickCheck()
		}
	}()

	// warm 池健康检查：每 HealthCheckInterval*2 分钟
	warmInterval := time.Duration(hc.cfg.HealthCheckInterval*2) * time.Minute
	warmTicker := time.NewTicker(warmInterval)
	go func() {
		for range warmTicker.C {
			hc.RunWarmCheck()
		}
	}()

	log.Printf("[health] 健康检查器已启动：快速检测 1分钟/次，完整检查 %d分钟/次，warm 池检查 %d分钟/次",
		hc.cfg.HealthCheckInterval, hc.cfg.HealthCheckInterval*2)
}
