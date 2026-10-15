package antigravity

import "testing"

func TestCallbackURLResolution(t *testing.T) {
	c := &Client{}

	// No hint: the loopback default, which is the only host Google has registered
	// for this client ID.
	if got := c.AuthRedirectURI(""); got != defaultCallbackURL {
		t.Errorf("want %q, got %q", defaultCallbackURL, got)
	}
	if got := c.AuthRedirectURI("http://localhost:20128/callback"); got != "http://localhost:20128/callback" {
		t.Errorf("hint ignored: %q", got)
	}

	// The exchange must reuse whatever URI was used to authorize.
	if got := c.ExchangeRedirectURI("http://localhost:4111/callback"); got != "http://localhost:4111/callback" {
		t.Errorf("explicit exchange URI ignored: %q", got)
	}
	if got := c.ExchangeRedirectURI(""); got != defaultCallbackURL {
		t.Errorf("exchange fallback not loopback: %q", got)
	}
}
