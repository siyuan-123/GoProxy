package fetcher

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"proxy-pool/storage"
)

// 代理来源定义
type Source struct {
	URL      string
	Protocol string // http 或 socks5
}

// 内置多个免费代理来源
var defaultSources = []Source{
	{"https://cdn.jsdelivr.net/gh/databay-labs/free-proxy-list/http.txt", "http"},
	{"https://cdn.jsdelivr.net/gh/databay-labs/free-proxy-list/socks5.txt", "socks5"},
	{"https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/all/data.txt", ""},
}

type Fetcher struct {
	sources []Source
	client  *http.Client
}

func New(httpURL, socks5URL string) *Fetcher {
	return &Fetcher{
		sources: defaultSources,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Fetch 从所有来源并发抓取代理
func (f *Fetcher) Fetch() ([]storage.Proxy, error) {
	type result struct {
		proxies []storage.Proxy
		source  Source
		err     error
	}

	ch := make(chan result, len(f.sources))
	for _, src := range f.sources {
		go func(s Source) {
			proxies, err := f.fetchFromURL(s.URL, s.Protocol)
			ch <- result{proxies: proxies, source: s, err: err}
		}(src)
	}

	var all []storage.Proxy
	seen := make(map[string]bool)
	for range f.sources {
		r := <-ch
		if r.err != nil {
			log.Printf("fetch %s error: %v", r.source.URL, r.err)
			continue
		}
		// 去重
		var deduped []storage.Proxy
		for _, p := range r.proxies {
			if !seen[p.Address] {
				seen[p.Address] = true
				deduped = append(deduped, p)
			}
		}
		log.Printf("fetched %d %s proxies from %s", len(deduped), r.source.Protocol, r.source.URL)
		all = append(all, deduped...)
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("no proxies fetched")
	}
	log.Printf("total fetched: %d proxies (deduped)", len(all))
	return all, nil
}

func (f *Fetcher) fetchFromURL(url, protocol string) ([]storage.Proxy, error) {
	resp, err := f.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	return parseProxyList(resp.Body, protocol)
}

func parseProxyList(r io.Reader, protocol string) ([]storage.Proxy, error) {
	var proxies []storage.Proxy
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		addr := line
		proto := protocol
		// 支持 protocol://host:port 格式
		if idx := strings.Index(line, "://"); idx != -1 {
			proto = line[:idx]
			addr = line[idx+3:]
			// socks4 当 socks5 处理
			if proto == "socks4" {
				proto = "socks5"
			}
		}
		parts := strings.Split(addr, ":")
		if len(parts) != 2 {
			continue
		}
		proxies = append(proxies, storage.Proxy{
			Address:  addr,
			Protocol: proto,
		})
	}
	return proxies, scanner.Err()
}
