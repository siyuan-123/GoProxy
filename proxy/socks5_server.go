package proxy

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"

	"goproxy/config"
	"goproxy/storage"
)

// relaySOCKS5Tunnel 双向转发 SOCKS5 隧道数据，带空闲超时保护，超时则记录代理失败。
func (s *SOCKS5Server) relaySOCKS5Tunnel(upstream, client net.Conn, p *storage.Proxy, target string) {
	cfg := config.Get()
	idleTimeout := time.Duration(cfg.TunnelIdleTimeout) * time.Second

	done := make(chan bool, 2)
	go func() { done <- idleTimeoutCopy(upstream, client, idleTimeout) }()
	go func() { done <- idleTimeoutCopy(client, upstream, idleTimeout) }()

	timedOut := <-done
	upstream.Close()
	client.Close()
	if t2 := <-done; t2 {
		timedOut = true
	}

	if timedOut {
		log.Printf("[socks5] ⏰ %s via %s 空闲超时 (>%ds)，代理异常，记录失败", target, p.Address, cfg.TunnelIdleTimeout)
		s.storage.RecordProxyUse(p.Address, false)
		s.handleSOCKS5ProxyFailure(p, cfg)
	}
}

// SOCKS5Server SOCKS5 协议服务器
type SOCKS5Server struct {
	storage         *storage.Storage
	cfg             *config.Config
	mode            string // "random" 或 "lowest-latency"
	port            string
	mu          sync.Mutex
	recentExits []string
}

// NewSOCKS5 创建 SOCKS5 服务器
func NewSOCKS5(s *storage.Storage, cfg *config.Config, mode string, port string) *SOCKS5Server {
	return &SOCKS5Server{
		storage: s,
		cfg:     cfg,
		mode:    mode,
		port:    port,
	}
}

// Start 启动 SOCKS5 服务器
func (s *SOCKS5Server) Start() error {
	modeDesc := "随机轮换"
	if s.mode == "lowest-latency" {
		modeDesc = "最低延迟"
	}
	authStatus := "无认证"
	if s.cfg.ProxyAuthEnabled {
		authStatus = fmt.Sprintf("需认证 (用户: %s)", s.cfg.ProxyAuthUsername)
	}
	log.Printf("socks5 server listening on %s [%s] [%s]", s.port, modeDesc, authStatus)

	listener, err := net.Listen("tcp", s.port)
	if err != nil {
		return err
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go s.handleConnection(conn)
	}
}

// handleConnection 处理 SOCKS5 连接
func (s *SOCKS5Server) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	// SOCKS5 握手
	if err := s.socks5Handshake(clientConn); err != nil {
		log.Printf("[socks5] handshake failed: %v", err)
		return
	}

	// 读取请求
	target, err := s.readSOCKS5Request(clientConn)
	if err != nil {
		log.Printf("[socks5] read request failed: %v", err)
		return
	}

	// 带重试的连接上游代理
	cfg := config.Get()
	tried := []string{}
	maxRetries := cfg.MaxRetry + 2

	for attempt := 0; attempt <= maxRetries; attempt++ {
		p, err := s.selectSOCKS5Proxy(tried, target)
		if err != nil {
			log.Printf("[socks5] no available socks5 upstream proxy: %v", err)
			s.sendSOCKS5Reply(clientConn, 0x01)
			return
		}

		tried = append(tried, p.Address)

		// 连接上游代理，测量建连延迟
		dialStart := time.Now()
		upstreamConn, err := s.dialViaProxy(p, target)
		dialLatency := time.Since(dialStart)
		if err != nil {
			log.Printf("[socks5] dial %s via %s (%s) failed: %v", target, p.Address, p.Protocol, err)
			s.storage.RecordProxyUse(p.Address, false)
			s.handleSOCKS5ProxyFailure(p, cfg)
			continue
		}

		// 记录建连延迟
		dialMs := int(dialLatency.Milliseconds())
		s.storage.UpdateServeLatency(p.Address, dialMs)

		// 发送成功响应
		if err := s.sendSOCKS5Reply(clientConn, 0x00); err != nil {
			upstreamConn.Close()
			return
		}

		s.storage.RecordProxyUse(p.Address, true)
		s.storage.ResetConsecutiveFails(p.Address)
		log.Printf("[socks5] %s via %s established (%dms)", target, p.Address, dialMs)

		// 双向转发数据（带空闲超时保护）
		s.relaySOCKS5Tunnel(upstreamConn, clientConn, p, target)
		return
	}

	s.sendSOCKS5Reply(clientConn, 0x01)
	log.Printf("[socks5] all proxies failed for %s", target)
}

// selectSOCKS5Proxy 根据使用模式选择 SOCKS5 上游代理
func (s *SOCKS5Server) selectSOCKS5Proxy(tried []string, targetHost string) (*storage.Proxy, error) {
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

	// 关键主机感知：只使用 S/A 级 + kiro_validated 的 SOCKS5 代理
	if cfg.IsCriticalHost(targetHost) {
		if cfg.CustomProxyMode == "mixed" && (cfg.CustomPriority || cfg.CustomFreePriority) {
			preferSource := "custom"
			if cfg.CustomFreePriority {
				preferSource = "free"
			}
			p, err := s.selectCriticalSOCKS5Proxy(tried, preferSource, avoidExits)
			if err == nil {
				updateLastExit(p.ExitIP)
				return p, nil
			}
			p, err = s.selectCriticalSOCKS5Proxy(tried, "", avoidExits)
			if err == nil {
				updateLastExit(p.ExitIP)
				return p, nil
			}
		} else {
			p, err := s.selectCriticalSOCKS5Proxy(tried, sourceFilter, avoidExits)
			if err == nil {
				updateLastExit(p.ExitIP)
				return p, nil
			}
		}
		log.Printf("[socks5] 关键主机 %s 无专属代理可用，降级到常规选取", targetHost)
	}

	// 混用 + 优先模式
	if cfg.CustomProxyMode == "mixed" && (cfg.CustomPriority || cfg.CustomFreePriority) {
		preferSource := "custom"
		if cfg.CustomFreePriority {
			preferSource = "free"
		}
		var p *storage.Proxy
		var err error
		if s.mode == "lowest-latency" {
			p, err = s.storage.GetLowestLatencyByProtocolExcludeFiltered("socks5", tried, preferSource)
		} else {
			p, err = s.storage.GetRandomByProtocolExcludeAvoidExitIPsFiltered("socks5", tried, preferSource, avoidExits)
		}
		if err == nil {
			updateLastExit(p.ExitIP)
			return p, nil
		}
		// fallback
		if s.mode == "lowest-latency" {
			p, err = s.storage.GetLowestLatencyByProtocolExcludeFiltered("socks5", tried, "")
		} else {
			p, err = s.storage.GetRandomByProtocolExcludeAvoidExitIPsFiltered("socks5", tried, "", avoidExits)
		}
		if err == nil {
			updateLastExit(p.ExitIP)
		}
		return p, err
	}

	if s.mode == "lowest-latency" {
		p, err := s.storage.GetLowestLatencyByProtocolExcludeFiltered("socks5", tried, sourceFilter)
		if err == nil {
			updateLastExit(p.ExitIP)
		}
		return p, err
	}
	p, err := s.storage.GetRandomByProtocolExcludeAvoidExitIPsFiltered("socks5", tried, sourceFilter, avoidExits)
	if err == nil {
		updateLastExit(p.ExitIP)
	}
	return p, err
}

// selectCriticalSOCKS5Proxy 为关键主机选取高质量 SOCKS5 代理，避开 recentExits 中的 IP 并随机化
func (s *SOCKS5Server) selectCriticalSOCKS5Proxy(tried []string, sourceFilter string, avoidExits []string) (*storage.Proxy, error) {
	proxies, err := s.storage.GetCriticalHostProxiesByProtocol("socks5", sourceFilter)
	if err != nil || len(proxies) == 0 {
		return nil, fmt.Errorf("no critical socks5 proxies")
	}

	triedMap := make(map[string]bool)
	for _, t := range tried {
		triedMap[t] = true
	}

	avoidSet := make(map[string]int, len(avoidExits))
	for i, ip := range avoidExits {
		avoidSet[ip] = i
	}

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
		sort.Slice(fallback, func(i, j int) bool {
			return avoidSet[fallback[i].ExitIP] < avoidSet[fallback[j].ExitIP]
		})
		p := fallback[0]
		return &p, nil
	}
	return nil, fmt.Errorf("all critical socks5 proxies tried")
}

// handleSOCKS5ProxyFailure 处理 SOCKS5 代理失败
func (s *SOCKS5Server) handleSOCKS5ProxyFailure(p *storage.Proxy, cfg *config.Config) {
	count, err := s.storage.IncrementConsecutiveFails(p.Address)
	if err != nil {
		removeOrDisableProxy(s.storage, p)
		return
	}
	if count >= cfg.ConsecutiveFailThreshold {
		log.Printf("[socks5] 代理 %s 连续失败 %d 次，踢出池子", p.Address, count)
		removeOrDisableProxy(s.storage, p)
	}
}

// socks5Handshake 处理 SOCKS5 握手
func (s *SOCKS5Server) socks5Handshake(conn net.Conn) error {
	buf := make([]byte, 257)

	// 读取客户端问候: [VER(1), NMETHODS(1), METHODS(1-255)]
	n, err := io.ReadAtLeast(conn, buf, 2)
	if err != nil {
		return err
	}

	version := buf[0]
	if version != 0x05 {
		return fmt.Errorf("unsupported SOCKS version: %d", version)
	}

	nmethods := int(buf[1])
	if n < 2+nmethods {
		if _, err := io.ReadFull(conn, buf[n:2+nmethods]); err != nil {
			return err
		}
	}

	// 检查是否需要认证
	needAuth := s.cfg.ProxyAuthEnabled
	methods := buf[2 : 2+nmethods]

	// 选择认证方式
	var selectedMethod byte = 0xFF // No acceptable methods
	if needAuth {
		// 需要用户名/密码认证 (0x02)
		for _, method := range methods {
			if method == 0x02 {
				selectedMethod = 0x02
				break
			}
		}
	} else {
		// 无需认证 (0x00)
		for _, method := range methods {
			if method == 0x00 {
				selectedMethod = 0x00
				break
			}
		}
	}

	// 发送方法选择: [VER(1), METHOD(1)]
	if _, err := conn.Write([]byte{0x05, selectedMethod}); err != nil {
		return err
	}

	if selectedMethod == 0xFF {
		return fmt.Errorf("no acceptable authentication method")
	}

	// 如果需要认证，进行用户名/密码认证
	if selectedMethod == 0x02 {
		if err := s.socks5Auth(conn); err != nil {
			return err
		}
	}

	return nil
}

// socks5Auth 处理 SOCKS5 用户名/密码认证
func (s *SOCKS5Server) socks5Auth(conn net.Conn) error {
	buf := make([]byte, 513)

	// 读取认证请求: [VER(1), ULEN(1), UNAME(1-255), PLEN(1), PASSWD(1-255)]
	n, err := io.ReadAtLeast(conn, buf, 2)
	if err != nil {
		return err
	}

	if buf[0] != 0x01 {
		return fmt.Errorf("unsupported auth version: %d", buf[0])
	}

	ulen := int(buf[1])
	if n < 2+ulen {
		if _, err := io.ReadFull(conn, buf[n:2+ulen]); err != nil {
			return err
		}
		n = 2 + ulen
	}

	username := string(buf[2 : 2+ulen])

	// 读取密码长度和密码
	if n < 2+ulen+1 {
		if _, err := io.ReadFull(conn, buf[n:2+ulen+1]); err != nil {
			return err
		}
		n = 2 + ulen + 1
	}

	plen := int(buf[2+ulen])
	if n < 2+ulen+1+plen {
		if _, err := io.ReadFull(conn, buf[n:2+ulen+1+plen]); err != nil {
			return err
		}
	}

	password := string(buf[2+ulen+1 : 2+ulen+1+plen])

	// 验证用户名和密码
	if username != s.cfg.ProxyAuthUsername || password != s.cfg.ProxyAuthPassword {
		// 认证失败: [VER(1), STATUS(1)]
		conn.Write([]byte{0x01, 0x01})
		return fmt.Errorf("authentication failed")
	}

	// 认证成功: [VER(1), STATUS(1)]
	if _, err := conn.Write([]byte{0x01, 0x00}); err != nil {
		return err
	}

	return nil
}

// readSOCKS5Request 读取 SOCKS5 请求
func (s *SOCKS5Server) readSOCKS5Request(conn net.Conn) (string, error) {
	buf := make([]byte, 262)

	// 读取请求: [VER(1), CMD(1), RSV(1), ATYP(1), DST.ADDR(variable), DST.PORT(2)]
	n, err := io.ReadAtLeast(conn, buf, 4)
	if err != nil {
		return "", err
	}

	if buf[0] != 0x05 {
		return "", fmt.Errorf("invalid version: %d", buf[0])
	}

	cmd := buf[1]
	if cmd != 0x01 { // 只支持 CONNECT
		s.sendSOCKS5Reply(conn, 0x07) // Command not supported
		return "", fmt.Errorf("unsupported command: %d", cmd)
	}

	atyp := buf[3]
	var host string
	var addrLen int

	switch atyp {
	case 0x01: // IPv4
		addrLen = 4
		if n < 4+addrLen+2 {
			if _, err := io.ReadFull(conn, buf[n:4+addrLen+2]); err != nil {
				return "", err
			}
		}
		host = fmt.Sprintf("%d.%d.%d.%d", buf[4], buf[5], buf[6], buf[7])
	case 0x03: // Domain name
		addrLen = int(buf[4])
		if n < 4+1+addrLen+2 {
			if _, err := io.ReadFull(conn, buf[n:4+1+addrLen+2]); err != nil {
				return "", err
			}
		}
		host = string(buf[5 : 5+addrLen])
	case 0x04: // IPv6
		addrLen = 16
		if n < 4+addrLen+2 {
			if _, err := io.ReadFull(conn, buf[n:4+addrLen+2]); err != nil {
				return "", err
			}
		}
		// 简化处理，直接转换
		host = net.IP(buf[4 : 4+addrLen]).String()
	default:
		s.sendSOCKS5Reply(conn, 0x08) // Address type not supported
		return "", fmt.Errorf("unsupported address type: %d", atyp)
	}

	// 读取端口
	portOffset := 4
	if atyp == 0x03 {
		portOffset = 5 + addrLen
	} else {
		portOffset = 4 + addrLen
	}
	port := binary.BigEndian.Uint16(buf[portOffset : portOffset+2])

	return fmt.Sprintf("%s:%d", host, port), nil
}

// sendSOCKS5Reply 发送 SOCKS5 响应
func (s *SOCKS5Server) sendSOCKS5Reply(conn net.Conn, rep byte) error {
	// [VER(1), REP(1), RSV(1), ATYP(1), BND.ADDR(variable), BND.PORT(2)]
	// 简化：使用 0.0.0.0:0
	reply := []byte{
		0x05,       // VER
		rep,        // REP: 0x00=成功, 0x01=一般失败, 0x07=命令不支持, 0x08=地址类型不支持
		0x00,       // RSV
		0x01,       // ATYP: IPv4
		0, 0, 0, 0, // BND.ADDR: 0.0.0.0
		0, 0, // BND.PORT: 0
	}
	_, err := conn.Write(reply)
	return err
}

// dialViaProxy 通过上游代理连接目标
func (s *SOCKS5Server) dialViaProxy(p *storage.Proxy, target string) (net.Conn, error) {
	cfg := config.Get()
	timeout := time.Duration(cfg.ProxyServeTimeout) * time.Second

	switch p.Protocol {
	case "http":
		return dialHTTPConnect(p.Address, target, timeout)

	case "socks5":
		// 使用 SOCKS5 代理
		dialer := &net.Dialer{Timeout: timeout}
		proxyConn, err := dialer.Dial("tcp", p.Address)
		if err != nil {
			return nil, err
		}

		// 握手+CONNECT 阶段设置整体 deadline，防止上游挂起
		proxyConn.SetDeadline(time.Now().Add(timeout))

		// SOCKS5 握手（无认证）
		if _, err := proxyConn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
			proxyConn.Close()
			return nil, err
		}

		handshake := make([]byte, 2)
		if _, err := io.ReadFull(proxyConn, handshake); err != nil {
			proxyConn.Close()
			return nil, err
		}

		if handshake[0] != 0x05 || handshake[1] != 0x00 {
			proxyConn.Close()
			return nil, fmt.Errorf("socks5 handshake failed")
		}

		// 发送 CONNECT 请求
		host, port, err := net.SplitHostPort(target)
		if err != nil {
			proxyConn.Close()
			return nil, err
		}

		// 构建请求
		req := []byte{0x05, 0x01, 0x00} // VER, CMD=CONNECT, RSV

		// 判断是 IP 还是域名
		if ip := net.ParseIP(host); ip != nil {
			if ip4 := ip.To4(); ip4 != nil {
				req = append(req, 0x01) // IPv4
				req = append(req, ip4...)
			} else {
				req = append(req, 0x04) // IPv6
				req = append(req, ip...)
			}
		} else {
			req = append(req, 0x03) // Domain
			req = append(req, byte(len(host)))
			req = append(req, []byte(host)...)
		}

		// 添加端口
		portNum := uint16(0)
		fmt.Sscanf(port, "%d", &portNum)
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, portNum)
		req = append(req, portBytes...)

		if _, err := proxyConn.Write(req); err != nil {
			proxyConn.Close()
			return nil, err
		}

		// 读取响应
		reply := make([]byte, 10)
		if _, err := io.ReadAtLeast(proxyConn, reply, 10); err != nil {
			proxyConn.Close()
			return nil, err
		}

		if reply[1] != 0x00 {
			proxyConn.Close()
			return nil, fmt.Errorf("socks5 connect failed, code: %d", reply[1])
		}

		// 握手完成，清除 deadline，后续数据转发由 relaySOCKS5Tunnel 管理超时
		proxyConn.SetDeadline(time.Time{})

		return proxyConn, nil

	default:
		return nil, fmt.Errorf("unsupported protocol: %s", p.Protocol)
	}
}
