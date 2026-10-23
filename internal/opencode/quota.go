package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"
)

// QuotaItem is one quota window shown by the dashboard.
type QuotaItem struct {
	Name                string  `json:"name"`
	ModelKey            string  `json:"modelKey,omitempty"`
	Used                int64   `json:"used"`
	Total               int64   `json:"total"`
	RemainingPercentage float64 `json:"remainingPercentage"`
	ResetAt             string  `json:"resetAt,omitempty"`
	Unlimited           bool    `json:"unlimited,omitempty"`
}

// AccountUsage is the quota snapshot for one OpenCode account.
type AccountUsage struct {
	AccountID string               `json:"accountId"`
	Email     string               `json:"email"`
	Plan      string               `json:"plan"`
	Message   string               `json:"message,omitempty"`
	Quotas    map[string]QuotaItem `json:"quotas"`
	FetchedAt time.Time            `json:"fetchedAt"`
}

var quotaWindows = []struct {
	key  string
	name string
}{
	{"rolling", "Rolling"},
	{"weekly", "Weekly"},
	{"monthly", "Monthly"},
}

// GetAccountUsage returns the OpenCode Go quota windows for one account.
func (c *Client) GetAccountUsage(ctx context.Context, acc *Account, force bool) (*AccountUsage, error) {
	if acc == nil {
		return nil, fmt.Errorf("account is required")
	}

	c.quotaMu.RLock()
	cached, hasCached := c.quotaCache[acc.ID]
	c.quotaMu.RUnlock()
	if !force && hasCached && cached != nil && time.Since(cached.FetchedAt) < 45*time.Second {
		return cached, nil
	}

	fresh, err := c.ensureFreshAccount(ctx, acc)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/zen/go/v1/usage", nil)
	if err != nil {
		return nil, fmt.Errorf("create usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+fresh.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.oauthUserAgent())
	if fresh.OrgID != "" {
		req.Header.Set("x-opencode-org-id", fresh.OrgID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute usage request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	usage := &AccountUsage{
		AccountID: fresh.ID,
		Email:     fresh.Email,
		Plan:      "OpenCode Go",
		Quotas:    map[string]QuotaItem{},
		FetchedAt: time.Now(),
	}

	if resp.StatusCode != http.StatusOK {
		message := consoleError(raw)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			usage.Message = message
			c.cacheUsage(usage)
			return usage, nil
		}
		return nil, fmt.Errorf("usage request failed (%d): %s", resp.StatusCode, message)
	}

	var payload struct {
		Usage map[string]struct {
			Status   string  `json:"status"`
			Percent  float64 `json:"percent"`
			ResetsAt string  `json:"resetsAt"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode usage response: %w", err)
	}

	for _, window := range quotaWindows {
		item, ok := payload.Usage[window.key]
		if !ok {
			continue
		}
		used := math.Max(0, math.Min(100, item.Percent))
		usage.Quotas[window.key] = QuotaItem{
			Name:                window.name,
			ModelKey:            window.key,
			Used:                int64(math.Round(used)),
			Total:               100,
			RemainingPercentage: 100 - used,
			ResetAt:             item.ResetsAt,
		}
	}
	if len(usage.Quotas) == 0 {
		usage.Message = "OpenCode Go usage response did not contain quota data"
	}

	c.cacheUsage(usage)
	return usage, nil
}

func (c *Client) cacheUsage(usage *AccountUsage) {
	c.quotaMu.Lock()
	c.quotaCache[usage.AccountID] = usage
	c.quotaMu.Unlock()
}

// GetAllUsage returns quota snapshots for every stored account.
func (c *Client) GetAllUsage(ctx context.Context, force bool) (map[string]*AccountUsage, error) {
	results := make(map[string]*AccountUsage)
	if c.accountStore == nil {
		return results, nil
	}
	accounts := c.accountStore.ListRaw()
	if len(accounts) == 0 {
		return results, nil
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, acc := range accounts {
		wg.Add(1)
		go func(a *Account) {
			defer wg.Done()
			reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			usage, err := c.GetAccountUsage(reqCtx, a, force)
			if err != nil || usage == nil {
				return
			}
			mu.Lock()
			results[a.ID] = usage
			mu.Unlock()
		}(acc)
	}
	wg.Wait()
	return results, nil
}
