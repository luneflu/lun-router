package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// European ISO-3166-1 alpha-2 country codes (DE handled separately as tier 0)
var EuropeanCountries = map[string]bool{
	"AL": true, "AD": true, "AT": true, "BA": true, "BE": true, "BG": true,
	"BY": true, "CH": true, "CY": true, "CZ": true, "DK": true, "EE": true,
	"ES": true, "FI": true, "FR": true, "GB": true, "GR": true, "HR": true,
	"HU": true, "IE": true, "IS": true, "IT": true, "LI": true, "LT": true,
	"LU": true, "LV": true, "MC": true, "MD": true, "ME": true, "MK": true,
	"MT": true, "NL": true, "NO": true, "PL": true, "PT": true, "RO": true,
	"RS": true, "RU": true, "SE": true, "SI": true, "SK": true, "SM": true,
	"UA": true, "VA": true,
}

const (
	GeonodeAPIURL     = "https://proxylist.geonode.com/api/proxy-list"
	ProxyCooldown     = 10 * time.Minute
	CacheTTL          = 2 * time.Hour
	ProxyDialTimeout  = 8 * time.Second
	ProxyTotalTimeout = 45 * time.Second
)

type Proxy struct {
	IP           string   `json:"ip"`
	Port         string   `json:"port"`
	Country      string   `json:"country"`
	City         string   `json:"city,omitempty"`
	Protocols    []string `json:"protocols"`
	UpTime       float64  `json:"upTime"`
	ResponseTime float64  `json:"responseTime"`
	Speed        float64  `json:"speed"`
	Latency      float64  `json:"latency"`
	Priority     int      `json:"priority"` // 0: Germany, 1: Europe, 2: Other
}

func (p *Proxy) Key() string {
	return net.JoinHostPort(p.IP, p.Port)
}

func (p *Proxy) EffectiveLatency() float64 {
	if p.ResponseTime > 0 {
		return p.ResponseTime
	}
	if p.Latency > 0 {
		return p.Latency
	}
	if p.Speed > 0 {
		return p.Speed
	}
	return 999999
}

func (p *Proxy) URL() string {
	hasSocks5 := false
	hasHTTP := false
	for _, proto := range p.Protocols {
		switch strings.ToLower(proto) {
		case "socks5":
			hasSocks5 = true
		case "http", "https":
			hasHTTP = true
		}
	}
	if hasSocks5 {
		return fmt.Sprintf("socks5://%s:%s", p.IP, p.Port)
	}
	if hasHTTP {
		return fmt.Sprintf("http://%s:%s", p.IP, p.Port)
	}
	return ""
}

func calculatePriority(country string) int {
	c := strings.ToUpper(strings.TrimSpace(country))
	if c == "DE" {
		return 0 // Germany priority first
	}
	if EuropeanCountries[c] {
		return 1 // Europe countries next
	}
	return 2 // Rest of world
}

type ProxyStatus struct {
	Enabled       bool     `json:"enabled"`
	AutoOnLimit   bool     `json:"auto_on_limit"`
	TotalCount    int      `json:"total_count"`
	GermanyCount  int      `json:"germany_count"`
	EuropeCount   int      `json:"europe_count"`
	OtherCount    int      `json:"other_count"`
	ActiveProxy   *Proxy   `json:"active_proxy,omitempty"`
	ActiveProxyURL string  `json:"active_proxy_url,omitempty"`
	FailedCount   int      `json:"failed_count"`
	LastFetch     string   `json:"last_fetch,omitempty"`
}

type ProxyManager struct {
	dataDir     string
	mu          sync.RWMutex
	enabled     bool
	autoOnLimit bool
	proxies     []*Proxy
	activeProxy *Proxy
	failedUntil map[string]time.Time
	lastFetch   time.Time
	clients     map[string]*http.Client
	httpClient  *http.Client
}

func NewProxyManager(dataDir string) *ProxyManager {
	pm := &ProxyManager{
		dataDir:     dataDir,
		autoOnLimit: true,
		failedUntil: make(map[string]time.Time),
		clients:     make(map[string]*http.Client),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
	_ = pm.loadCache()
	return pm
}

func (pm *ProxyManager) IsEnabled() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.enabled
}

func (pm *ProxyManager) SetEnabled(enabled bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.enabled = enabled
	if !enabled {
		pm.activeProxy = nil
	}
}

func (pm *ProxyManager) ActiveProxy() *Proxy {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.activeProxy
}

// NextProxy picks the highest-priority, healthy proxy (Germany -> Europe -> Rest, by uptime then speed)
func (pm *ProxyManager) NextProxy() *Proxy {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.proxies) == 0 {
		return nil
	}

	now := time.Now()
	for _, p := range pm.proxies {
		key := p.Key()
		if expire, failed := pm.failedUntil[key]; failed && now.Before(expire) {
			continue
		}
		if p.URL() == "" {
			continue
		}
		pm.activeProxy = p
		return p
	}

	// If all proxies are in cooldown, reset cooldowns and take first
	pm.failedUntil = make(map[string]time.Time)
	for _, p := range pm.proxies {
		if p.URL() != "" {
			pm.activeProxy = p
			return p
		}
	}

	return nil
}

func (pm *ProxyManager) MarkSuccess(p *Proxy) {
	if p == nil {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.activeProxy = p
	delete(pm.failedUntil, p.Key())
}

func (pm *ProxyManager) MarkFailed(p *Proxy, reason string) {
	if p == nil {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	log.Printf("[opencode-proxy] marking proxy %s (%s) failed: %s", p.Key(), p.Country, reason)
	pm.failedUntil[p.Key()] = time.Now().Add(ProxyCooldown)
	if pm.activeProxy != nil && pm.activeProxy.Key() == p.Key() {
		pm.activeProxy = nil
	}
}

func (pm *ProxyManager) ClientForProxy(p *Proxy) (*http.Client, error) {
	if p == nil {
		return http.DefaultClient, nil
	}
	proxyURLStr := p.URL()
	if proxyURLStr == "" {
		return nil, fmt.Errorf("no supported protocol for proxy %s", p.Key())
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if client, ok := pm.clients[proxyURLStr]; ok {
		return client, nil
	}

	parsed, err := url.Parse(proxyURLStr)
	if err != nil {
		return nil, fmt.Errorf("parse proxy url %s: %w", proxyURLStr, err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(parsed),
		DialContext: (&net.Dialer{
			Timeout:   ProxyDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   ProxyDialTimeout,
		ResponseHeaderTimeout: 25 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   ProxyTotalTimeout,
	}
	pm.clients[proxyURLStr] = client
	return client, nil
}

func (pm *ProxyManager) GetActiveClient() *http.Client {
	pm.mu.RLock()
	p := pm.activeProxy
	pm.mu.RUnlock()

	if p == nil {
		return nil
	}
	c, err := pm.ClientForProxy(p)
	if err != nil {
		return nil
	}
	return c
}

// Fetch loads proxies from Geonode: Germany first, then Europe countries, sorted by uptime, then speed
func (pm *ProxyManager) Fetch(ctx context.Context) error {
	var collected []*Proxy
	seen := make(map[string]bool)

	// 1. Fetch Germany explicitly with high limit
	deProxies, err := pm.fetchGeonode(ctx, "country=DE&limit=500&sort_by=upTime&sort_type=desc")
	if err == nil {
		for _, p := range deProxies {
			key := p.Key()
			if !seen[key] && p.URL() != "" {
				seen[key] = true
				collected = append(collected, p)
			}
		}
	}

	// 2. Fetch European and global proxies sorted by uptime
	allProxies, err := pm.fetchGeonode(ctx, "limit=500&sort_by=upTime&sort_type=desc")
	if err == nil {
		for _, p := range allProxies {
			key := p.Key()
			if !seen[key] && p.URL() != "" {
				seen[key] = true
				collected = append(collected, p)
			}
		}
	}

	if len(collected) == 0 {
		return fmt.Errorf("no valid proxies fetched from Geonode")
	}

	// Sort by Priority (0: DE, 1: Europe, 2: Other) -> UpTime desc -> ResponseTime/Speed asc
	sort.Slice(collected, func(i, j int) bool {
		pi := collected[i]
		pj := collected[j]
		if pi.Priority != pj.Priority {
			return pi.Priority < pj.Priority
		}
		if pi.UpTime != pj.UpTime {
			return pi.UpTime > pj.UpTime
		}
		return pi.EffectiveLatency() < pj.EffectiveLatency()
	})

	pm.mu.Lock()
	pm.proxies = collected
	pm.lastFetch = time.Now()
	pm.mu.Unlock()

	_ = pm.saveCache()
	log.Printf("[opencode-proxy] fetched %d proxies (DE: %d, Europe: %d, Other: %d)",
		len(collected),
		countPriority(collected, 0),
		countPriority(collected, 1),
		countPriority(collected, 2),
	)
	return nil
}

func countPriority(list []*Proxy, prio int) int {
	c := 0
	for _, p := range list {
		if p.Priority == prio {
			c++
		}
	}
	return c
}

type geonodeItem struct {
	IP           string          `json:"ip"`
	Port         json.RawMessage `json:"port"`
	Country      string          `json:"country"`
	City         string          `json:"city"`
	Protocols    []string        `json:"protocols"`
	UpTime       float64         `json:"upTime"`
	ResponseTime float64         `json:"responseTime"`
	Speed        float64         `json:"speed"`
	Latency      float64         `json:"latency"`
}

func (pm *ProxyManager) fetchGeonode(ctx context.Context, query string) ([]*Proxy, error) {
	reqURL := fmt.Sprintf("%s?protocols=http,https,socks5&%s", GeonodeAPIURL, query)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; LunRouter/1.0)")
	req.Header.Set("Accept", "application/json")

	resp, err := pm.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("geonode returned %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		Data []geonodeItem `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	var list []*Proxy
	for _, item := range res.Data {
		portStr := strings.Trim(string(item.Port), "\"")
		if portStr == "" {
			continue
		}
		p := &Proxy{
			IP:           item.IP,
			Port:         portStr,
			Country:      item.Country,
			City:         item.City,
			Protocols:    item.Protocols,
			UpTime:       item.UpTime,
			ResponseTime: item.ResponseTime,
			Speed:        item.Speed,
			Latency:      item.Latency,
			Priority:     calculatePriority(item.Country),
		}
		list = append(list, p)
	}
	return list, nil
}

func (pm *ProxyManager) Status() ProxyStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var activeURL string
	if pm.activeProxy != nil {
		activeURL = pm.activeProxy.URL()
	}

	var lastFetchStr string
	if !pm.lastFetch.IsZero() {
		lastFetchStr = pm.lastFetch.Format(time.RFC3339)
	}

	return ProxyStatus{
		Enabled:        pm.enabled,
		AutoOnLimit:    pm.autoOnLimit,
		TotalCount:     len(pm.proxies),
		GermanyCount:   countPriority(pm.proxies, 0),
		EuropeCount:    countPriority(pm.proxies, 1),
		OtherCount:     countPriority(pm.proxies, 2),
		ActiveProxy:    pm.activeProxy,
		ActiveProxyURL: activeURL,
		FailedCount:    len(pm.failedUntil),
		LastFetch:      lastFetchStr,
	}
}

func (pm *ProxyManager) cachePath() string {
	if pm.dataDir == "" {
		return ""
	}
	return filepath.Join(pm.dataDir, "opencode_proxies.json")
}

func (pm *ProxyManager) saveCache() error {
	path := pm.cachePath()
	if path == "" {
		return nil
	}
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	data, err := json.MarshalIndent(map[string]any{
		"last_fetch": pm.lastFetch,
		"proxies":    pm.proxies,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (pm *ProxyManager) loadCache() error {
	path := pm.cachePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var payload struct {
		LastFetch time.Time `json:"last_fetch"`
		Proxies   []*Proxy  `json:"proxies"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	if len(payload.Proxies) > 0 {
		pm.mu.Lock()
		pm.proxies = payload.Proxies
		pm.lastFetch = payload.LastFetch
		pm.mu.Unlock()
	}
	return nil
}

func (pm *ProxyManager) EnsureLoaded(ctx context.Context) {
	pm.mu.RLock()
	count := len(pm.proxies)
	stale := time.Since(pm.lastFetch) > CacheTTL
	pm.mu.RUnlock()

	if count == 0 || stale {
		_ = pm.Fetch(ctx)
	}
}
