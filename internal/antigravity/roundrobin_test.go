package antigravity

import (
	"testing"
	"time"
)

// newTestClient builds a client with accounts loaded, but no network access.
func newTestClient(t *testing.T, accounts ...*Account) *Client {
	t.Helper()
	c := &Client{
		store: &AccountStore{
			accounts: make(map[string]*Account),
		},
		quotaCache: make(map[string]*AccountUsage),
		exhausted:  make(map[string]time.Time),
		models:     append([]string{}, CatalogModels...),
	}
	for _, acc := range accounts {
		c.store.accounts[acc.ID] = acc
	}
	return c
}

func acct(id string) *Account {
	return &Account{ID: id, Email: id + "@example.com", IsActive: true}
}

// setQuota caches a balance for one account. modelKey may be a concrete model id
// or a family bucket such as "gemini_session".
func setQuota(c *Client, accountID, modelKey string, pct float64) {
	usage, ok := c.quotaCache[accountID]
	if !ok || usage == nil {
		usage = &AccountUsage{AccountID: accountID, Quotas: map[string]QuotaItem{}}
		c.quotaCache[accountID] = usage
	}
	usage.Quotas[modelKey] = QuotaItem{ModelKey: modelKey, RemainingPercentage: pct, Total: 1000}
	usage.FetchedAt = time.Now()
}

// TestSelectsRichestAccount is the core requirement: the account with the most
// remaining credit is used first.
func TestSelectsRichestAccount(t *testing.T) {
	c := newTestClient(t, acct("poor"), acct("rich"), acct("mid"))
	setQuota(c, "poor", "gemini_session", 5)
	setQuota(c, "rich", "gemini_session", 90)
	setQuota(c, "mid", "gemini_session", 50)

	got, ok := c.GetActiveForModel("gemini-3.8-flash")
	if !ok {
		t.Fatal("expected an account")
	}
	if got.ID != "rich" {
		t.Errorf("picked %s, want rich (90 percent remaining)", got.ID)
	}
}

// TestModelBucketBeatsFamilyBucket checks the ranking preference: a balance
// reported for the exact model is a tighter signal than the family bucket.
func TestModelBucketBeatsFamilyBucket(t *testing.T) {
	c := newTestClient(t, acct("a"), acct("b"))
	// a has more session credit, but b has more of the model we are asking for.
	setQuota(c, "a", "gemini_session", 80)
	setQuota(c, "a", "gemini-3.8-flash", 10)
	setQuota(c, "b", "gemini_session", 20)
	setQuota(c, "b", "gemini-3.8-flash", 70)

	got, _ := c.GetActiveForModel("gemini-3.8-flash")
	if got.ID != "b" {
		t.Errorf("picked %s, want b (exact model bucket wins)", got.ID)
	}
}

// TestSessionBucketBeatsWeekly covers the fallback order between the two family
// buckets: the 5h session window is what gates a request.
func TestSessionBucketBeatsWeekly(t *testing.T) {
	c := newTestClient(t, acct("a"), acct("b"))
	setQuota(c, "a", "gemini_weekly", 95)
	setQuota(c, "a", "gemini_session", 10)
	setQuota(c, "b", "gemini_weekly", 30)
	setQuota(c, "b", "gemini_session", 60)

	got, _ := c.GetActiveForModel("gemini-3.8-flash")
	if got.ID != "b" {
		t.Errorf("picked %s, want b (session bucket decides)", got.ID)
	}
}

// TestScarcestBucketWhenMappingUnknown covers a model whose family maps to no
// known bucket: the account is scored by its scarcest reported bucket.
func TestScarcestBucketWhenMappingUnknown(t *testing.T) {
	c := newTestClient(t, acct("tight"), acct("roomy"))
	// Neither account reports a gemini bucket, so the fallback applies.
	setQuota(c, "tight", "claude_gpt_session", 5)
	setQuota(c, "tight", "claude_gpt_weekly", 90)
	setQuota(c, "roomy", "claude_gpt_session", 40)
	setQuota(c, "roomy", "claude_gpt_weekly", 45)

	got, _ := c.GetActiveForModel("gemini-3.8-flash")
	if got.ID != "roomy" {
		t.Errorf("picked %s, want roomy (scarcest bucket 40 > 5)", got.ID)
	}
}

// TestTiedBalancesRotate is the "prefer other eligible accounts when balances
// tie" requirement: equal accounts must share load, not race for one.
func TestTiedBalancesRotate(t *testing.T) {
	c := newTestClient(t, acct("a"), acct("b"))
	setQuota(c, "a", "gemini_session", 60)
	setQuota(c, "b", "gemini_session", 60)

	seen := map[string]int{}
	for i := 0; i < 6; i++ {
		got, ok := c.GetActiveForModel("gemini-3.8-flash")
		if !ok {
			t.Fatal("expected an account")
		}
		seen[got.ID]++
	}
	if seen["a"] != 3 || seen["b"] != 3 {
		t.Errorf("tied accounts should alternate evenly, got %v", seen)
	}
}

// TestSlightlyDifferentBalancesDoNotRotate makes sure the tie tolerance does not
// turn distinct balances into rotation - the richer account must keep winning.
func TestSlightlyDifferentBalancesDoNotRotate(t *testing.T) {
	c := newTestClient(t, acct("a"), acct("b"))
	setQuota(c, "a", "gemini_session", 80)
	setQuota(c, "b", "gemini_session", 40)

	for i := 0; i < 4; i++ {
		got, _ := c.GetActiveForModel("gemini-3.8-flash")
		if got.ID != "a" {
			t.Fatalf("iteration %d picked %s, want a every time", i, got.ID)
		}
	}
}

// TestExhaustedAccountNeverSelected checks that a drained account is skipped even
// when it is the only one with a reported balance.
func TestExhaustedAccountNeverSelected(t *testing.T) {
	c := newTestClient(t, acct("drained"), acct("ok"))
	setQuota(c, "drained", "gemini_session", 0)
	setQuota(c, "ok", "gemini_session", 55)
	// Mark the drained account as out of quota for this model until the reset.
	c.MarkExhausted("drained", "gemini-3.8-flash", time.Now().Add(time.Hour))

	for i := 0; i < 3; i++ {
		got, ok := c.GetActiveForModel("gemini-3.8-flash")
		if !ok {
			t.Fatal("expected an account")
		}
		if got.ID == "drained" {
			t.Fatal("selected an exhausted account")
		}
	}
}

// TestNoQuotaDataFallsBackToRotation covers a cold cache: selection must still
// return every eligible account in turn rather than failing or always picking the
// same one.
func TestNoQuotaDataFallsBackToRotation(t *testing.T) {
	c := newTestClient(t, acct("a"), acct("b"))

	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		got, ok := c.GetActiveForModel("gemini-3.8-flash")
		if !ok {
			t.Fatal("expected an account")
		}
		seen[got.ID]++
	}
	if seen["a"] != 2 || seen["b"] != 2 {
		t.Errorf("cold cache should rotate, got %v", seen)
	}
}

// TestNoActiveAccounts reports failure rather than a nil account.
func TestNoActiveAccounts(t *testing.T) {
	c := newTestClient(t)
	if _, ok := c.GetActiveForModel("gemini-3.8-flash"); ok {
		t.Error("expected no account when none are active")
	}
}

// TestAllAccountsExhaustedStillReturnsOne keeps the previous behaviour: when
// every account is drained the caller still gets an account so the upstream error
// is reported instead of a misleading "no accounts" failure.
func TestAllAccountsExhaustedStillReturnsOne(t *testing.T) {
	c := newTestClient(t, acct("a"))
	c.MarkExhausted("a", "gemini-3.8-flash", time.Now().Add(time.Hour))

	if _, ok := c.GetActiveForModel("gemini-3.8-flash"); !ok {
		t.Error("expected a fallback account so the error surfaces")
	}
}

// TestUnlimitedBucketOutranksFinite ensures an unlimited plan is never scored
// below a partially-used one.
func TestUnlimitedBucketOutranksFinite(t *testing.T) {
	c := newTestClient(t, acct("finite"), acct("unlimited"))
	setQuota(c, "finite", "gemini_session", 99)
	c.quotaCache["unlimited"] = &AccountUsage{
		AccountID: "unlimited",
		Quotas:    map[string]QuotaItem{"gemini_session": {Unlimited: true}},
		FetchedAt: time.Now(),
	}

	got, _ := c.GetActiveForModel("gemini-3.8-flash")
	if got.ID != "unlimited" {
		t.Errorf("picked %s, want unlimited", got.ID)
	}
}

// TestCreditsLockedPreferenceOrder pins the lookup order directly.
func TestCreditsLockedPreferenceOrder(t *testing.T) {
	c := newTestClient(t, acct("a"))
	c.quotaCache["a"] = &AccountUsage{
		AccountID: "a",
		Quotas: map[string]QuotaItem{
			"gemini-3.8-flash":   {RemainingPercentage: 11},
			"gemini_session":     {RemainingPercentage: 22},
			"gemini_weekly":      {RemainingPercentage: 33},
			"claude_gpt_session": {RemainingPercentage: 44},
		},
	}

	if score, known := c.creditsLocked("a", "gemini-3.8-flash", "gemini"); !known || score != 11 {
		t.Errorf("exact model bucket: got (%v, %v), want (11, true)", score, known)
	}
	if score, _ := c.creditsLocked("a", "gemini-9-nope", "gemini"); score != 22 {
		t.Errorf("session bucket: got %v, want 22", score)
	}
	c.quotaCache["a"].Quotas = map[string]QuotaItem{
		"gemini_weekly":      {RemainingPercentage: 33},
		"claude_gpt_session": {RemainingPercentage: 44},
	}
	if score, _ := c.creditsLocked("a", "gemini-9-nope", "gemini"); score != 33 {
		t.Errorf("weekly bucket: got %v, want 33", score)
	}
	// No family match: scarcest bucket wins.
	c.quotaCache["a"].Quotas = map[string]QuotaItem{
		"gemini_weekly":      {RemainingPercentage: 33},
		"claude_gpt_session": {RemainingPercentage: 4},
	}
	if score, _ := c.creditsLocked("a", "unknown-model", ""); score != 4 {
		t.Errorf("scarcest fallback: got %v, want 4", score)
	}
	// Unknown account.
	if _, known := c.creditsLocked("missing", "gemini-3.8-flash", "gemini"); known {
		t.Error("an account with no cached quota must report known=false")
	}
}
