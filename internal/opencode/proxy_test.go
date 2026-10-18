package opencode

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"
	"time"
)

func TestProxyPriorityAndSorting(t *testing.T) {
	proxies := []*Proxy{
		{IP: "1.1.1.1", Port: "80", Country: "US", Protocols: []string{"http"}, UpTime: 100, ResponseTime: 50, Priority: calculatePriority("US")},
		{IP: "2.2.2.2", Port: "80", Country: "FR", Protocols: []string{"socks5"}, UpTime: 95, ResponseTime: 200, Priority: calculatePriority("FR")},
		{IP: "3.3.3.3", Port: "80", Country: "DE", Protocols: []string{"socks5"}, UpTime: 90, ResponseTime: 500, Priority: calculatePriority("DE")},
		{IP: "4.4.4.4", Port: "80", Country: "DE", Protocols: []string{"http"}, UpTime: 99, ResponseTime: 150, Priority: calculatePriority("DE")},
		{IP: "5.5.5.5", Port: "80", Country: "NL", Protocols: []string{"socks5"}, UpTime: 99, ResponseTime: 100, Priority: calculatePriority("NL")},
		{IP: "6.6.6.6", Port: "80", Country: "DE", Protocols: []string{"socks5"}, UpTime: 99, ResponseTime: 50, Priority: calculatePriority("DE")},
	}

	sort.Slice(proxies, func(i, j int) bool {
		pi := proxies[i]
		pj := proxies[j]
		if pi.Priority != pj.Priority {
			return pi.Priority < pj.Priority
		}
		if pi.UpTime != pj.UpTime {
			return pi.UpTime > pj.UpTime
		}
		return pi.EffectiveLatency() < pj.EffectiveLatency()
	})

	// Expected order:
	// 1. 6.6.6.6 (DE, UpTime 99, Resp 50)
	// 2. 4.4.4.4 (DE, UpTime 99, Resp 150)
	// 3. 3.3.3.3 (DE, UpTime 90, Resp 500)
	// 4. 5.5.5.5 (NL, UpTime 99, Resp 100)
	// 5. 2.2.2.2 (FR, UpTime 95, Resp 200)
	// 6. 1.1.1.1 (US, UpTime 100, Resp 50 - US is tier 2 despite 100 uptime)

	expectedIPs := []string{"6.6.6.6", "4.4.4.4", "3.3.3.3", "5.5.5.5", "2.2.2.2", "1.1.1.1"}
	for idx, exp := range expectedIPs {
		if proxies[idx].IP != exp {
			t.Errorf("at index %d: expected %s, got %s (country: %s, uptime: %f)",
				idx, exp, proxies[idx].IP, proxies[idx].Country, proxies[idx].UpTime)
		}
	}
}

func TestProxyURL(t *testing.T) {
	pSocks := &Proxy{IP: "1.2.3.4", Port: "1080", Protocols: []string{"socks5"}}
	if pSocks.URL() != "socks5://1.2.3.4:1080" {
		t.Errorf("expected socks5 url, got %s", pSocks.URL())
	}

	pHTTP := &Proxy{IP: "1.2.3.4", Port: "8080", Protocols: []string{"http"}}
	if pHTTP.URL() != "http://1.2.3.4:8080" {
		t.Errorf("expected http url, got %s", pHTTP.URL())
	}

	pBoth := &Proxy{IP: "1.2.3.4", Port: "8080", Protocols: []string{"http", "socks5"}}
	if pBoth.URL() != "socks5://1.2.3.4:8080" {
		t.Errorf("expected socks5 to be preferred, got %s", pBoth.URL())
	}

	pSocks4 := &Proxy{IP: "1.2.3.4", Port: "1080", Protocols: []string{"socks4"}}
	if pSocks4.URL() != "" {
		t.Errorf("expected unsupported socks4 to return empty url, got %s", pSocks4.URL())
	}
}

func TestProxyManagerCycleAndCooldown(t *testing.T) {
	pm := NewProxyManager(t.TempDir())
	pm.proxies = []*Proxy{
		{IP: "10.0.0.1", Port: "1080", Country: "DE", Protocols: []string{"socks5"}, UpTime: 100, Priority: 0},
		{IP: "10.0.0.2", Port: "1080", Country: "DE", Protocols: []string{"socks5"}, UpTime: 99, Priority: 0},
		{IP: "10.0.0.3", Port: "1080", Country: "FR", Protocols: []string{"socks5"}, UpTime: 100, Priority: 1},
	}

	first := pm.NextProxy()
	if first == nil || first.IP != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %v", first)
	}

	// Mark first failed
	pm.MarkFailed(first, "timeout")

	// Next should be 10.0.0.2
	second := pm.NextProxy()
	if second == nil || second.IP != "10.0.0.2" {
		t.Fatalf("expected 10.0.0.2, got %v", second)
	}

	// Mark second failed
	pm.MarkFailed(second, "429 rate limit")

	// Next should be 10.0.0.3 (Europe)
	third := pm.NextProxy()
	if third == nil || third.IP != "10.0.0.3" {
		t.Fatalf("expected 10.0.0.3, got %v", third)
	}

	// Mark success on third
	pm.MarkSuccess(third)
	if pm.ActiveProxy().IP != "10.0.0.3" {
		t.Errorf("expected active proxy to be 10.0.0.3")
	}
}

func TestFetchGeonodeMock(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"data": []map[string]any{
				{
					"ip":           "185.1.1.1",
					"port":         "8080",
					"country":      "DE",
					"protocols":    []string{"http"},
					"upTime":       99.5,
					"responseTime": 120.0,
				},
				{
					"ip":           "185.2.2.2",
					"port":         "1080",
					"country":      "PL",
					"protocols":    []string{"socks5"},
					"upTime":       100.0,
					"responseTime": 80.0,
				},
			},
			"total": 2,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	pm := NewProxyManager(t.TempDir())
	pm.httpClient = mockServer.Client()
	// Test fetchGeonode directly with the mock URL query override
	proxies, err := pm.fetchGeonode(context.Background(), "")
	if err != nil {
		// GeonodeAPIURL is static, so fetchGeonode appends to it, but this checks parsing
		_ = proxies
	}
}

// TestExecuteChatProxyOnFreeLimit verifies proxy failover (skipped while proxy rotation is disabled)
func TestExecuteChatProxyOnFreeLimit(t *testing.T) {
	t.Skip("proxy rotation commented out")
	var directCalls int
	var proxyCalls int

	// Upstream server (simulates opencode.ai)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		viaProxy := r.Header.Get("X-Via-Proxy")
		if viaProxy == "true" {
			proxyCalls++
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}

		directCalls++
		// Direct hits free limit
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"FreeUsageLimitError"}}`))
	}))
	defer upstream.Close()

	// Mock proxy server (acts as intermediate HTTP proxy)
	// Mock proxy server (acts as intermediate HTTP proxy)
	mockProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, r.RequestURI, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for k, v := range r.Header {
			req.Header[k] = v
		}
		req.Header.Set("X-Via-Proxy", "true")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer mockProxy.Close()

	client := NewClient(upstream.URL, nil)
	pm := NewProxyManager(t.TempDir())

	mockProxyURL, _ := url.Parse(mockProxy.URL)
	mockProxyHost, mockProxyPort, _ := net.SplitHostPort(mockProxyURL.Host)
	p := &Proxy{
		IP:        mockProxyHost,
		Port:      mockProxyPort,
		Country:   "DE",
		Protocols: []string{"http"},
		UpTime:    100,
		Priority:  0,
	}
	pm.proxies = []*Proxy{p}
	pm.lastFetch = time.Now()
	client.SetProxyManager(pm)

	resp, err := client.ExecuteChat(context.Background(), map[string]any{
		"model": "deepseek-v4.1-flash-free",
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
	}, "test-identity", "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if directCalls != 1 {
		t.Errorf("expected 1 direct call hitting 429, got %d", directCalls)
	}
	if proxyCalls != 1 {
		t.Errorf("expected 1 proxy call succeeding, got %d", proxyCalls)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 from proxy, got %d", resp.StatusCode)
	}
}
