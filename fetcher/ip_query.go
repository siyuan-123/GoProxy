package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// IPQueryLimiter 全局IP查询限流器
var IPQueryLimiter *rate.Limiter

// InitIPQueryLimiter 初始化限流器
func InitIPQueryLimiter(rps int) {
	IPQueryLimiter = rate.NewLimiter(rate.Limit(rps), rps*2)
}

// GetExitIPInfo 通过代理获取出口 IP 和地理位置（多源降级）
func GetExitIPInfo(client *http.Client) (string, string) {
	// 等待限流令牌
	if IPQueryLimiter != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := IPQueryLimiter.Wait(ctx); err != nil {
			return "", ""
		}
	}

	// 优先级1：ip-api.com
	if ip, loc := tryIPAPI(client); ip != "" {
		return ip, loc
	}

	// 优先级2：ipapi.co
	if ip, loc := tryIPAPICo(client); ip != "" {
		return ip, loc
	}

	// 优先级3：ipinfo.io
	if ip, loc := tryIPInfo(client); ip != "" {
		return ip, loc
	}

	// 优先级4：仅获取IP
	if ip := tryHTTPBinIP(client); ip != "" {
		return ip, "UNKNOWN"
	}

	return "", ""
}

// tryIPAPI 尝试 ip-api.com
func tryIPAPI(client *http.Client) (string, string) {
	resp, err := client.Get("http://ip-api.com/json/?fields=status,country,countryCode,city,query")
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	var result struct {
		Status      string `json:"status"`
		Query       string `json:"query"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		City        string `json:"city"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.Status != "success" {
		return "", ""
	}

	location := result.CountryCode
	if result.City != "" {
		location = fmt.Sprintf("%s %s", result.CountryCode, result.City)
	}

	return result.Query, location
}

// tryIPAPICo 尝试 ipapi.co
func tryIPAPICo(client *http.Client) (string, string) {
	resp, err := client.Get("https://ipapi.co/json/")
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	var result struct {
		IP          string `json:"ip"`
		City        string `json:"city"`
		CountryCode string `json:"country_code"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", ""
	}

	location := result.CountryCode
	if result.City != "" {
		location = fmt.Sprintf("%s %s", result.CountryCode, result.City)
	}

	return result.IP, location
}

// tryIPInfo 尝试 ipinfo.io
func tryIPInfo(client *http.Client) (string, string) {
	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	var result struct {
		IP      string `json:"ip"`
		City    string `json:"city"`
		Country string `json:"country"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", ""
	}

	location := result.Country
	if result.City != "" {
		location = fmt.Sprintf("%s %s", result.Country, result.City)
	}

	return result.IP, location
}

// tryHTTPBinIP 尝试 httpbin（仅获取IP）
func tryHTTPBinIP(client *http.Client) string {
	resp, err := client.Get("https://httpbin.org/ip")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		Origin string `json:"origin"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	return result.Origin
}
