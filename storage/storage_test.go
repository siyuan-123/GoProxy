package storage

import "testing"

func TestPreferDistinctExitIPs(t *testing.T) {
	t.Parallel()

	proxies := []Proxy{
		{Address: "a", ExitIP: "1.1.1.1"},
		{Address: "b", ExitIP: "1.1.1.1"},
		{Address: "c", ExitIP: "2.2.2.2"},
		{Address: "d", ExitIP: ""},
	}

	result := preferDistinctExitIPs(proxies)
	if len(result) != len(proxies) {
		t.Fatalf("expected %d proxies, got %d", len(proxies), len(result))
	}
	if result[0].Address != "a" {
		t.Fatalf("expected first unique proxy to stay first, got %s", result[0].Address)
	}
	if result[1].Address != "c" {
		t.Fatalf("expected second unique exit ip to be promoted, got %s", result[1].Address)
	}
}

func TestFilterByAvoidExitIPs(t *testing.T) {
	t.Parallel()

	proxies := []Proxy{
		{Address: "a", ExitIP: "1.1.1.1"},
		{Address: "b", ExitIP: "2.2.2.2"},
		{Address: "c", ExitIP: ""},
		{Address: "d", ExitIP: "3.3.3.3"},
	}

	// 避开单个 IP
	filtered := filterByAvoidExitIPs(proxies, []string{"1.1.1.1"})
	if len(filtered) != 3 {
		t.Fatalf("expected 3 proxies after filtering, got %d", len(filtered))
	}
	for _, p := range filtered {
		if p.ExitIP == "1.1.1.1" {
			t.Fatalf("expected filtered result to avoid exit ip, got %+v", p)
		}
	}

	// 避开多个 IP
	filtered2 := filterByAvoidExitIPs(proxies, []string{"1.1.1.1", "2.2.2.2"})
	if len(filtered2) != 2 {
		t.Fatalf("expected 2 proxies after filtering, got %d", len(filtered2))
	}

	// 全部被避开时，按 LRU 顺序回退（索引小的优先）
	allAvoided := []Proxy{
		{Address: "x", ExitIP: "1.1.1.1"},
		{Address: "y", ExitIP: "2.2.2.2"},
	}
	fallback := filterByAvoidExitIPs(allAvoided, []string{"1.1.1.1", "2.2.2.2"})
	if len(fallback) != 2 {
		t.Fatalf("expected fallback to return all proxies, got %d", len(fallback))
	}
	if fallback[0].ExitIP != "1.1.1.1" {
		t.Fatalf("expected LRU fallback: oldest (1.1.1.1) first, got %s", fallback[0].ExitIP)
	}

	// 空 avoidExitIPs 不过滤
	noFilter := filterByAvoidExitIPs(proxies, nil)
	if len(noFilter) != len(proxies) {
		t.Fatalf("expected no filtering with nil avoidExitIPs, got %d", len(noFilter))
	}
}
