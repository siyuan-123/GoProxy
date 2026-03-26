package checker

import (
	"log"
	"time"

	"proxy-pool/config"
	"proxy-pool/storage"
	"proxy-pool/validator"
)

type Checker struct {
	storage *storage.Storage
}

func New(s *storage.Storage, _ *validator.Validator, _ *config.Config) *Checker {
	return &Checker{storage: s}
}

func (c *Checker) Start() {
	go func() {
		for {
			cfg := config.Get()
			time.Sleep(time.Duration(cfg.CheckInterval) * time.Minute)
			c.run()
		}
	}()
	log.Printf("health checker started, interval: %d min", config.Get().CheckInterval)
}

func (c *Checker) run() {
	log.Println("[checker] start health check...")

	proxies, err := c.storage.GetAll()
	if err != nil {
		log.Printf("[checker] get proxies error: %v", err)
		return
	}
	if len(proxies) == 0 {
		log.Println("[checker] no proxies to check")
		return
	}

	// 每次用最新配置创建 validator
	cfg := config.Get()
	validate := validator.New(cfg.ValidateConcurrency, cfg.ValidateTimeout, cfg.ValidateURL)

	log.Printf("[checker] checking %d proxies...", len(proxies))
	results := validate.ValidateAll(proxies)

	valid, invalid := 0, 0
	for _, r := range results {
		if r.Valid {
			valid++
		} else {
			invalid++
			if err := c.storage.Delete(r.Proxy.Address); err != nil {
				log.Printf("[checker] delete error: %v", err)
			}
		}
	}

	count, _ := c.storage.Count()
	log.Printf("[checker] done: valid=%d invalid(deleted)=%d remaining=%d", valid, invalid, count)
}
