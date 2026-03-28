package validator

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/proxy"
	"goproxy/config"
	"goproxy/storage"
)

type Validator struct {
	concurrency   int
	timeout       time.Duration
	validateURL   string
	maxResponseMs int
}

func concurrencyBuffer(total, concurrency int) int {
	if total < concurrency*10 {
		return total
	}
	return concurrency * 10
}

func New(concurrency, timeoutSec int, validateURL string) *Validator {
	cfg := config.Get()
	maxMs := 0
	if cfg != nil {
		maxMs = cfg.MaxResponseMs
	}
	return &Validator{
		concurrency:   concurrency,
		timeout:       time.Duration(timeoutSec) * time.Second,
		validateURL:   validateURL,
		maxResponseMs: maxMs,
	}
}

type Result struct {
	Proxy        storage.Proxy
	Valid        bool
	Latency      time.Duration
	ExitIP       string
	ExitLocation string
}

// getExitIPInfo 通过代理获取出口 IP 和地理位置
func getExitIPInfo(client *http.Client) (string, string) {
	// 使用 ip-api.com 返回 JSON 格式的 IP 信息
	resp, err := client.Get("http://ip-api.com/json/?fields=status,country,countryCode,city,query")
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	var result struct {
		Status      string `json:"status"`
		Query       string `json:"query"`       // IP 地址
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		City        string `json:"city"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.Status != "success" {
		return "", ""
	}

	// 返回格式：IP, "国家代码 城市"
	location := result.CountryCode
	if result.City != "" {
		location = fmt.Sprintf("%s %s", result.CountryCode, result.City)
	}
	
	return result.Query, location
}

// ValidateAll 并发验证所有代理，返回验证结果
func (v *Validator) ValidateAll(proxies []storage.Proxy) []Result {
	var results []Result
	for r := range v.ValidateStream(proxies) {
		results = append(results, r)
	}
	return results
}

// ValidateStream 并发验证，边验证边通过 channel 返回结果
func (v *Validator) ValidateStream(proxies []storage.Proxy) <-chan Result {
	ch := make(chan Result, concurrencyBuffer(len(proxies), v.concurrency))
	sem := make(chan struct{}, v.concurrency)
	var wg sync.WaitGroup

	go func() {
		for _, p := range proxies {
			wg.Add(1)
			sem <- struct{}{}
			go func(px storage.Proxy) {
				defer wg.Done()
				defer func() { <-sem }()
				valid, latency, exitIP, exitLocation := v.ValidateOne(px)
				ch <- Result{Proxy: px, Valid: valid, Latency: latency, ExitIP: exitIP, ExitLocation: exitLocation}
			}(p)
		}
		wg.Wait()
		close(ch)
	}()

	return ch
}

// ValidateOne 验证单个代理是否可用，返回是否有效、延迟、出口IP和地理位置
func (v *Validator) ValidateOne(p storage.Proxy) (bool, time.Duration, string, string) {
	var client *http.Client
	var err error

	switch p.Protocol {
	case "http":
		client, err = newHTTPClient(p.Address, v.timeout)
	case "socks5":
		client, err = newSOCKS5Client(p.Address, v.timeout)
	default:
		log.Printf("unknown protocol %s for %s", p.Protocol, p.Address)
		return false, 0, "", ""
	}

	if err != nil {
		return false, 0, "", ""
	}

	start := time.Now()
	resp, err := client.Get(v.validateURL)
	latency := time.Since(start)
	if err != nil {
		return false, 0, "", ""
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// 验证状态码（200 或 204 都接受）
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return false, latency, "", ""
	}

	// 响应时间过滤
	if v.maxResponseMs > 0 && latency > time.Duration(v.maxResponseMs)*time.Millisecond {
		return false, latency, "", ""
	}

	// 获取出口 IP 和地理位置（仅在验证通过时）
	exitIP, exitLocation := getExitIPInfo(client)
	
	// 必须能获取到出口信息
	if exitIP == "" || exitLocation == "" {
		return false, latency, exitIP, exitLocation
	}
	
	// 过滤中国大陆出口（香港的countryCode是HK，不是CN）
	if len(exitLocation) >= 2 {
		countryCode := exitLocation[:2]
		if countryCode == "CN" {
			// 中国大陆出口，直接拒绝
			return false, latency, exitIP, exitLocation
		}
	}

	return true, latency, exitIP, exitLocation
}

func newHTTPClient(address string, timeout time.Duration) (*http.Client, error) {
	proxyURL, err := url.Parse(fmt.Sprintf("http://%s", address))
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: timeout,
	}, nil
}

func newSOCKS5Client(address string, timeout time.Duration) (*http.Client, error) {
	dialer, err := proxy.SOCKS5("tcp", address, nil, proxy.Direct)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport: &http.Transport{
			Dial: dialer.Dial,
		},
		Timeout: timeout,
	}, nil
}
