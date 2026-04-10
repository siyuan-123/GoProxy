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

	validCount := 0
	removeCount := 0
	updateCount := 0

	for result := range hc.validator.ValidateStream(proxies) {
		if result.Valid {
			validCount++
			latencyMs := int(result.Latency.Milliseconds())
			if err := hc.storage.UpdateExitInfo(result.Proxy.Address, result.ExitIP, result.ExitLocation, latencyMs); err == nil {
				updateCount++
			}
			// 健康检查通过：重置 fail_count 和 consecutive_fails，让恢复的代理重新可用
			hc.storage.ResetFail(result.Proxy.Address)
			hc.storage.ResetConsecutiveFails(result.Proxy.Address)
			// 更新 Kiro 验证信息（如果 HTTPS 探测有延迟数据）
			if result.HTTPSLatency > 0 {
				httpsMs := int(result.HTTPSLatency.Milliseconds())
				hc.storage.UpdateKiroValidation(result.Proxy.Address, httpsMs)
			}
		} else {
			hc.storage.IncrementFailCount(result.Proxy.Address)
			// 连续失败计数
			consecutiveCount, _ := hc.storage.IncrementConsecutiveFails(result.Proxy.Address)
			// 连续失败达阈值 或 总失败次数 >= 3 时踢出
			shouldRemove := result.Proxy.FailCount+1 >= 3 || consecutiveCount >= hc.cfg.ConsecutiveFailThreshold
			if shouldRemove {
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
	log.Printf("[health] 完成: 验证%d 有效%d 更新%d 移除%d 耗时%v",
		len(proxies), validCount, updateCount, removeCount, elapsed)
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

	log.Printf("[health] 健康检查器已启动：快速检测 1分钟/次，完整检查 %d分钟/次", hc.cfg.HealthCheckInterval)
}
