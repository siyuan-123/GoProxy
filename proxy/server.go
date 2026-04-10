package proxy

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"goproxy/config"
	"goproxy/storage"

	"golang.org/x/net/proxy"
)

const maxRecentExits = 20

type Server struct {
	storage     *storage.Storage
	cfg         *config.Config
	mode        string // "random" 或 "lowest-latency"
	port        string
	mu          sync.Mutex
	recentExits []string
}

func New(s *storage.Storage, cfg *config.Config, mode string, port string) *Server {
	return &Server{
		storage: s,
		cfg:     cfg,
		mode:    mode,
		port:    port,
	}
}

func (s *Server) Start() error {
	modeDesc := "随机轮换"
	if s.mode == "lowest-latency" {
		modeDesc = "最低延迟"
	}
	authStatus := "无认证"
	if s.cfg.ProxyAuthEnabled {
		authStatus = fmt.Sprintf("需认证 (用户: %s)", s.cfg.ProxyAuthUsername)
	}
	log.Printf("proxy server listening on %s [%s] [%s]", s.port, modeDesc, authStatus)
	return http.ListenAndServe(s.port, s)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 认证检查（如果启用）
	if s.cfg.ProxyAuthEnabled {
		if !s.checkAuth(r) {
			w.Header().Set("Proxy-Authenticate", `Basic realm="GoProxy"`)
			http.Error(w, "Proxy Authentication Required", http.StatusProxyAuthRequired)
			return
		}
	}

	if r.Method == http.MethodConnect {
		s.handleTunnel(w, r)
	} else {
		s.handleHTTP(w, r)
	}
}

// checkAuth 验证代理 Basic Auth
func (s *Server) checkAuth(r *http.Request) bool {
	auth := r.Header.Get("Proxy-Authorization")
	if auth == "" {
		return false
	}

	// 解析 Basic Auth
	const prefix = "Basic "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}

	decoded, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return false
	}

	credentials := strings.SplitN(string(decoded), ":", 2)
	if len(credentials) != 2 {
		return false
	}

	username := credentials[0]
	password := credentials[1]

	// 验证用户名和密码
	usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(s.cfg.ProxyAuthUsername)) == 1
	passwordHash := fmt.Sprintf("%x", sha256.Sum256([]byte(password)))
	passwordMatch := subtle.ConstantTimeCompare([]byte(passwordHash), []byte(s.cfg.ProxyAuthPasswordHash)) == 1

	return usernameMatch && passwordMatch
}

// selectProxy 根据使用模式和选择策略获取代理
func (s *Server) selectProxy(tried []string, lowestLatency bool, targetHost string) (*storage.Proxy, error) {
	cfg := config.Get()
	sourceFilter := sourceFilterFromMode(cfg.CustomProxyMode)

	s.mu.Lock()
	avoidExits := make([]string, len(s.recentExits))
	copy(avoidExits, s.recentExits)
	s.mu.Unlock()

	updateLastExit := func(exitIP string) {
		if exitIP == "" {
			return
		}
		s.mu.Lock()
		// LRU: 移除已存在的相同 IP，追加到末尾
		for i, ip := range s.recentExits {
			if ip == exitIP {
				s.recentExits = append(s.recentExits[:i], s.recentExits[i+1:]...)
				break
			}
		}
		s.recentExits = append(s.recentExits, exitIP)
		if len(s.recentExits) > maxRecentExits {
			s.recentExits = s.recentExits[len(s.recentExits)-maxRecentExits:]
		}
		s.mu.Unlock()
	}

	// 关键主机感知：只使用 S/A 级 + kiro_validated 的代理
	if cfg.IsCriticalHost(targetHost) {
		// 混用 + 优先模式下，关键主机也优先从优先源选取
		if cfg.CustomProxyMode == "mixed" && (cfg.CustomPriority || cfg.CustomFreePriority) {
			preferSource := "custom"
			if cfg.CustomFreePriority {
				preferSource = "free"
			}
			p, err := s.selectCriticalProxy(tried, preferSource, cfg, avoidExits)
			if err == nil {
				updateLastExit(p.ExitIP)
				return p, nil
			}
			// 优先源无关键代理，fallback 到全部关键代理
			p, err = s.selectCriticalProxy(tried, "", cfg, avoidExits)
			if err == nil {
				updateLastExit(p.ExitIP)
				return p, nil
			}
		} else {
			p, err := s.selectCriticalProxy(tried, sourceFilter, cfg, avoidExits)
			if err == nil {
				updateLastExit(p.ExitIP)
				return p, nil
			}
		}
		log.Printf("[proxy] 关键主机 %s 无专属代理可用，降级到常规选取", targetHost)
	}

	// 混用 + 优先模式：先尝试优先源，无可用则 fallback 全部
	if cfg.CustomProxyMode == "mixed" && (cfg.CustomPriority || cfg.CustomFreePriority) {
		preferSource := "custom"
		if cfg.CustomFreePriority {
			preferSource = "free"
		}
		var p *storage.Proxy
		var err error
		if lowestLatency {
			p, err = s.storage.GetLowestLatencyExcludeFiltered(tried, preferSource)
		} else {
			p, err = s.storage.GetRandomExcludeAvoidExitIPsFiltered(tried, preferSource, avoidExits)
		}
		if err == nil {
			updateLastExit(p.ExitIP)
			return p, nil
		}
		// fallback 到全部
		if lowestLatency {
			p, err = s.storage.GetLowestLatencyExcludeFiltered(tried, "")
		} else {
			p, err = s.storage.GetRandomExcludeAvoidExitIPsFiltered(tried, "", avoidExits)
		}
		if err == nil {
			updateLastExit(p.ExitIP)
		}
		return p, err
	}

	if lowestLatency {
		p, err := s.storage.GetLowestLatencyExcludeFiltered(tried, sourceFilter)
		if err == nil {
			updateLastExit(p.ExitIP)
		}
		return p, err
	}
	p, err := s.storage.GetRandomExcludeAvoidExitIPsFiltered(tried, sourceFilter, avoidExits)
	if err == nil {
		updateLastExit(p.ExitIP)
	}
	return p, err
}

// selectCriticalProxy 为关键主机选取高质量代理，避开 recentExits 中的 IP 并随机化
func (s *Server) selectCriticalProxy(tried []string, sourceFilter string, cfg *config.Config, avoidExits []string) (*storage.Proxy, error) {
	proxies, err := s.storage.GetCriticalHostProxies(sourceFilter)
	if err != nil || len(proxies) == 0 {
		return nil, fmt.Errorf("no critical host proxies")
	}

	triedMap := make(map[string]bool)
	for _, t := range tried {
		triedMap[t] = true
	}

	avoidSet := make(map[string]int, len(avoidExits))
	for i, ip := range avoidExits {
		avoidSet[ip] = i
	}

	// 分两组：优先使用不在 recentExits 中的出口 IP 的代理
	var preferred, fallback []storage.Proxy
	for _, p := range proxies {
		if triedMap[p.Address] {
			continue
		}
		if _, found := avoidSet[p.ExitIP]; found && p.ExitIP != "" {
			fallback = append(fallback, p)
		} else {
			preferred = append(preferred, p)
		}
	}

	if len(preferred) > 0 {
		p := preferred[rand.Intn(len(preferred))]
		return &p, nil
	}
	if len(fallback) > 0 {
		// 按 LRU 排序：索引越小 = 越早使用 = 优先选择
		sort.Slice(fallback, func(i, j int) bool {
			return avoidSet[fallback[i].ExitIP] < avoidSet[fallback[j].ExitIP]
		})
		p := fallback[0]
		return &p, nil
	}
	return nil, fmt.Errorf("all critical proxies tried")
}

// handleProxyFailure 处理代理失败：增加连续失败计数，达阈值则踢出
func (s *Server) handleProxyFailure(p *storage.Proxy, cfg *config.Config) {
	count, err := s.storage.IncrementConsecutiveFails(p.Address)
	if err != nil {
		removeOrDisableProxy(s.storage, p)
		return
	}
	if count >= cfg.ConsecutiveFailThreshold {
		log.Printf("[proxy] 代理 %s 连续失败 %d 次，踢出池子", p.Address, count)
		removeOrDisableProxy(s.storage, p)
	}
}

// removeOrDisableProxy 根据代理来源决定删除或禁用
func removeOrDisableProxy(store *storage.Storage, p *storage.Proxy) {
	if p.Source == "custom" {
		store.DisableProxy(p.Address)
	} else {
		store.Delete(p.Address)
	}
}

// sourceFilterFromMode 根据使用模式返回来源过滤值
func sourceFilterFromMode(mode string) string {
	switch mode {
	case "custom_only":
		return "custom"
	case "free_only":
		return "free"
	default:
		return "" // mixed
	}
}

// handleHTTP 处理普通 HTTP 请求（带自动重试）
func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	var tried []string
	for attempt := 0; attempt <= cfg.MaxRetry; attempt++ {
		p, err := s.selectProxy(tried, s.mode == "lowest-latency", r.Host)
		if err != nil {
			http.Error(w, "no available proxy", http.StatusServiceUnavailable)
			return
		}

		tried = append(tried, p.Address)

		client, err := s.buildClient(p)
		if err != nil {
			removeOrDisableProxy(s.storage, p)
			continue
		}

		// 转发请求（使用完整 URL，上游代理通过 client transport 设置）
		req, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
		if err != nil {
			continue
		}
		req.Header = r.Header.Clone()
		req.Header.Del("Proxy-Connection")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[proxy] %s via %s failed: %v", r.RequestURI, p.Address, err)
			s.storage.RecordProxyUse(p.Address, false)
			s.handleProxyFailure(p, cfg)
			continue
		}
		defer resp.Body.Close()

		// 写回响应
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		s.storage.RecordProxyUse(p.Address, true)
		s.storage.ResetConsecutiveFails(p.Address)
		if resp.StatusCode == 429 {
			log.Printf("[proxy] ⚠️  429 %s via %s (protocol=%s)", r.RequestURI, p.Address, p.Protocol)
		} else {
			log.Printf("[proxy] %s via %s -> %d", r.RequestURI, p.Address, resp.StatusCode)
		}
		return
	}

	http.Error(w, "all proxies failed", http.StatusBadGateway)
}

// handleTunnel 处理 HTTPS CONNECT 隧道（带自动重试）
func (s *Server) handleTunnel(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	var tried []string
	for attempt := 0; attempt <= cfg.MaxRetry; attempt++ {
		p, err := s.selectProxy(tried, s.mode == "lowest-latency", r.Host)
		if err != nil {
			http.Error(w, "no available proxy", http.StatusServiceUnavailable)
			return
		}

		tried = append(tried, p.Address)

		dialStart := time.Now()
		conn, err := s.dialViaProxy(p, r.Host)
		dialLatency := time.Since(dialStart)
		if err != nil {
			log.Printf("[tunnel] dial %s via %s failed (%v), removing", r.Host, p.Address, err)
			s.storage.RecordProxyUse(p.Address, false)
			s.handleProxyFailure(p, cfg)
			continue
		}

		// 记录建连延迟
		dialMs := int(dialLatency.Milliseconds())
		s.storage.UpdateServeLatency(p.Address, dialMs)
		s.storage.RecordProxyUse(p.Address, true)
		s.storage.ResetConsecutiveFails(p.Address)

		// 告知客户端隧道建立
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			conn.Close()
			http.Error(w, "hijack not supported", http.StatusInternalServerError)
			return
		}
		clientConn, _, err := hijacker.Hijack()
		if err != nil {
			conn.Close()
			return
		}

		fmt.Fprintf(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		log.Printf("[tunnel] %s via %s established (%dms)", r.Host, p.Address, dialMs)

		// 双向转发（带空闲超时保护）
		go s.relayTunnel(conn, clientConn, p, r.Host)
		return
	}

	http.Error(w, "all proxies failed", http.StatusBadGateway)
}

func (s *Server) dialViaProxy(p *storage.Proxy, host string) (net.Conn, error) {
	cfg := config.Get()
	timeout := time.Duration(cfg.ProxyServeTimeout) * time.Second
	switch p.Protocol {
	case "http":
		return dialHTTPConnect(p.Address, host, timeout)
	case "socks5":
		dialer, err := proxy.SOCKS5("tcp", p.Address, nil, &net.Dialer{Timeout: timeout})
		if err != nil {
			return nil, err
		}
		conn, err := dialer.Dial("tcp", host)
		if err != nil {
			return nil, err
		}
		return conn, nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", p.Protocol)
	}
}

func (s *Server) buildClient(p *storage.Proxy) (*http.Client, error) {
	cfg := config.Get()
	timeout := time.Duration(cfg.ProxyServeTimeout) * time.Second
	switch p.Protocol {
	case "http":
		proxyURL, err := url.Parse(fmt.Sprintf("http://%s", p.Address))
		if err != nil {
			return nil, err
		}
		return &http.Client{
			Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
			Timeout:   timeout,
		}, nil
	case "socks5":
		dialer, err := proxy.SOCKS5("tcp", p.Address, nil, &net.Dialer{Timeout: timeout})
		if err != nil {
			return nil, err
		}
		return &http.Client{
			Transport: &http.Transport{Dial: dialer.Dial},
			Timeout:   timeout,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", p.Protocol)
	}
}

func transfer(dst io.WriteCloser, src io.ReadCloser) {
	defer dst.Close()
	defer src.Close()
	io.Copy(dst, src)
}

// idleTimeoutCopy 带空闲超时的数据拷贝。
// 每次读到数据后重置超时计时，若 idleTimeout 内无任何数据则返回 true。
func idleTimeoutCopy(dst, src net.Conn, idleTimeout time.Duration) bool {
	buf := make([]byte, 32*1024)
	for {
		src.SetReadDeadline(time.Now().Add(idleTimeout))
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return false
			}
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return true
			}
			return false
		}
	}
}

// relayTunnel 双向转发隧道数据，带空闲超时保护，超时则记录代理失败。
func (s *Server) relayTunnel(upstream, client net.Conn, p *storage.Proxy, host string) {
	cfg := config.Get()
	idleTimeout := time.Duration(cfg.TunnelIdleTimeout) * time.Second

	done := make(chan bool, 2)
	go func() { done <- idleTimeoutCopy(upstream, client, idleTimeout) }()
	go func() { done <- idleTimeoutCopy(client, upstream, idleTimeout) }()

	// 等待任一方向完成
	timedOut := <-done
	// 关闭双向连接，触发另一个 goroutine 结束
	upstream.Close()
	client.Close()
	if t2 := <-done; t2 {
		timedOut = true
	}

	if timedOut {
		log.Printf("[tunnel] ⏰ %s via %s 空闲超时 (>%ds)，代理异常，记录失败", host, p.Address, cfg.TunnelIdleTimeout)
		// 不再调用 RecordProxyUse(false)，因为建连时已计为一次成功使用，
		// 避免同一连接被双重计数（use_count 虚高）
		s.handleProxyFailure(p, cfg)
	}
}
