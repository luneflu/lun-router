package zai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"lun-router/internal/chatgpt"
)

// Aliyun captcha constants, taken from the chat.z.ai frontend bundle:
//
//	Rh = {REGION:"sgp", PREFIX:"no8xfe", SCENE_ID:"didk33e0", MODE:"popup"}
//	fD = "https://o.alicdn.com/captcha-frontend/aliyunCaptcha/AliyunCaptcha.js"
const (
	captchaRegion   = "sgp"
	captchaPrefix   = "no8xfe"
	captchaSceneID  = "didk33e0"
	captchaScript   = "https://o.alicdn.com/captcha-frontend/aliyunCaptcha/AliyunCaptcha.js"
	captchaLogo     = "https://z-cdn.chatglm.cn/z-ai/static/logo.svg"
	captchaElement  = "lun-captcha-element"
	captchaTrigger  = "lun-captcha-trigger"
	captchaPageURL  = "https://chat.z.ai/"
	captchaMintWait = 30 * time.Second
	// captchaMaxAge bounds how long a minted param may sit unused. Tokens are
	// single-use, so a stale one is worthless anyway; this only stops a pool
	// from growing unbounded when traffic pauses.
	captchaMaxAge = 3 * time.Minute
)

// Credentials is the session state the native client needs. The browser mints
// it once per browser session; everything else is plain HTTP.
type Credentials struct {
	Token    string // guest bearer token, also present as the `token` cookie
	UserID   string // extracted from the JWT payload
	DeviceID string // localStorage._arms_uid, sent as x-device-id
}

// captchaParam is one single-use Aliyun verification token as decoded.
type captchaParam struct {
	CertifyID     string    `json:"certifyId"`
	SceneID       string    `json:"sceneId"`
	IsSign        bool      `json:"isSign"`
	SecurityToken string    `json:"securityToken"`
	Raw           string    `json:"-"`
	MintedAt      time.Time `json:"-"`
}

// mintScript initialises the Aliyun SDK in the page and resolves with the
// verification param. It mirrors the frontend's own initialisation, including
// the hidden element and trigger the SDK requires.
const mintScript = `(async function(){
  const inject = () => new Promise((res, rej) => {
    if (window.initAliyunCaptcha) return res();
    window.AliyunCaptchaConfig = { region: %q, prefix: %q };
    const s = document.createElement('script');
    s.src = %q;
    s.onload = () => res();
    s.onerror = () => rej(new Error('captcha script load failed'));
    document.head.appendChild(s);
  });
  try {
    await inject();
  } catch (e) { return 'ERR:script:' + String(e && e.message || e); }
  if (typeof window.initAliyunCaptcha !== 'function') return 'ERR:init_missing';

  for (const [id, css] of [
    [%q, 'position:absolute;left:-99999px;top:-99999px;width:0;height:0;overflow:hidden;'],
    [%q, 'position:absolute;left:-99999px;top:-99999px;width:100px;height:40px;']
  ]) {
    if (!document.getElementById(id)) {
      const d = document.createElement('div');
      d.id = id; d.style.cssText = css; document.body.appendChild(d);
    }
  }

  return await new Promise((resolve) => {
    let settled = false;
    const done = v => { if (!settled) { settled = true; resolve(v); } };
    setTimeout(() => done('ERR:timeout'), 25000);
    try {
      window.initAliyunCaptcha({
        SceneId: %q, mode: 'popup',
        element: '#' + %q, button: '#' + %q,
        captchaLogoImg: %q, language: 'en',
        timeout: 10000, delayBeforeSuccess: false,
        success: p => done('OK:' + p),
        fail: e => done('ERR:fail:' + JSON.stringify(e)),
        onError: e => done('ERR:onerror:' + JSON.stringify(e)),
        onClose: () => done('ERR:closed'),
        getInstance: () => setTimeout(() => {
          const b = document.getElementById(%q);
          if (b) b.click();
        }, 30)
      });
    } catch (e) { done('ERR:throw:' + String(e && e.message || e)); }
  });
})()`

// buildMintScript renders mintScript with the constants above, so the values
// appear exactly once in the source.
func buildMintScript() string {
	return fmt.Sprintf(mintScript,
		captchaRegion, captchaPrefix, captchaScript,
		captchaElement, captchaTrigger,
		captchaSceneID, captchaElement, captchaTrigger,
		captchaLogo, captchaTrigger,
	)
}

// creditedScript captures the token and device id the page already holds, so a
// mint run also refreshes the session credentials.
const credentialScript = `JSON.stringify({
  token: (function(){ try { return localStorage.getItem('token') || ''; } catch(e){ return ''; } })(),
  arms:  (function(){ try { return localStorage.getItem('_arms_uid') || ''; } catch(e){ return ''; } })()
})`

// MintResult carries everything one browser visit produces.
type MintResult struct {
	Param       string
	Credentials Credentials
}

// mintCaptcha loads the page, harvests credentials and mints one captcha param.
func (c *Client) mintCaptcha(ctx context.Context) (MintResult, error) {
	c.browserMu.Lock()
	defer c.browserMu.Unlock()

	b, err := c.ensureBrowser(ctx)
	if err != nil {
		return MintResult{}, err
	}

	script := buildMintScript()
	raw, err := b.evaluate(ctx, script, true)
	if err != nil {
		// A failed evaluate usually means the page navigated or died; drop the
		// browser so the next attempt starts clean.
		c.dropBrowserLocked()
		return MintResult{}, fmt.Errorf("captcha mint: %w", err)
	}

	switch {
	case strings.HasPrefix(raw, "OK:"):
		param := strings.TrimPrefix(raw, "OK:")
		if param == "" {
			return MintResult{}, errors.New("captcha mint returned an empty param")
		}
	case strings.HasPrefix(raw, "ERR:"):
		c.dropBrowserLocked()
		return MintResult{}, fmt.Errorf("captcha mint failed: %s", strings.TrimPrefix(raw, "ERR:"))
	default:
		return MintResult{}, fmt.Errorf("captcha mint returned an unexpected value: %.120s", raw)
	}

	creds, err := c.readCredentials(ctx, b)
	if err != nil {
		return MintResult{}, err
	}
	return MintResult{Param: raw[len("OK:"):], Credentials: creds}, nil
}

// readCredentials pulls the guest token and device id out of the live page.
func (c *Client) readCredentials(ctx context.Context, b *browser) (Credentials, error) {
	raw, err := b.evaluate(ctx, credentialScript, false)
	if err != nil {
		return Credentials{}, fmt.Errorf("read credentials: %w", err)
	}
	var got struct {
		Token string `json:"token"`
		Arms  string `json:"arms"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return Credentials{}, fmt.Errorf("decode credentials: %w", err)
	}

	creds := Credentials{Token: got.Token, DeviceID: got.Arms}
	if creds.Token == "" {
		// The page stores the token after its own /api/v1/auths/ call. Falling
		// back to a direct request keeps the browser from being a hard
		// dependency for authentication.
		tok, uid, err := fetchGuestToken(ctx)
		if err != nil {
			return Credentials{}, fmt.Errorf("no token on page and guest fetch failed: %w", err)
		}
		creds.Token, creds.UserID = tok, uid
		return creds, nil
	}
	creds.UserID = userIDFromToken(creds.Token)
	return creds, nil
}

// ensureBrowser lazily launches the browser and loads chat.z.ai. The page is
// kept resident because warm mints cost ~350-900ms against ~7s cold.
//
// Callers must hold browserMu.
func (c *Client) ensureBrowser(ctx context.Context) (*browser, error) {
	if c.browserAliveLocked() {
		return c.browser, nil
	}
	c.dropBrowserLocked()

	executable := c.cfg.BrowserPath
	if executable == "" {
		executable = c.installer.DetectedExecutable()
	}
	if executable == "" {
		return nil, errors.New("no browser executable found - install the browser from the provider page first")
	}

	b, err := launchBrowser(ctx, executable, c.debugPort, c.profileDir)
	if err != nil {
		return nil, err
	}
	if err := b.navigate(ctx, captchaPageURL); err != nil {
		b.Close()
		return nil, err
	}
	// The SDK is injected lazily by the page itself; the app needs a moment to
	// store its token before a mint can succeed.
	select {
	case <-ctx.Done():
		b.Close()
		return nil, ctx.Err()
	case <-time.After(3 * time.Second):
	}

	c.browser = b
	return b, nil
}

// browserAliveLocked reports whether the attached browser is still usable.
// Callers must hold browserMu: browserMu is not reentrant, so a locking variant
// called from inside the mint path deadlocks the request.
func (c *Client) browserAliveLocked() bool {
	return c.browser != nil && !c.browser.closed && c.browser.conn != nil
}

// dropBrowserLocked tears the browser down. Callers must hold browserMu.
func (c *Client) dropBrowserLocked() {
	if c.browser != nil {
		c.browser.Close()
		c.browser = nil
	}
}

// CaptchaAvailable reports whether a browser binary exists to mint with.
func (c *Client) CaptchaAvailable() bool {
	if c.cfg.BrowserPath != "" {
		_, err := os.Stat(c.cfg.BrowserPath)
		return err == nil
	}
	return c.installer.DetectedExecutable() != ""
}

// decodeParam decodes a captcha param for diagnostics; the raw string is what
// gets sent, this only exists so the UI can show what is inside it.
func decodeParam(param string) (captchaParam, error) {
	raw, err := base64.StdEncoding.DecodeString(param)
	if err != nil {
		return captchaParam{}, err
	}
	var p captchaParam
	if err := json.Unmarshal(raw, &p); err != nil {
		return captchaParam{}, err
	}
	p.Raw = param
	return p, nil
}

// installerFor keeps the chatgpt package's installer typing out of the rest of
// this file.
func installerFor(dataDir string) *chatgpt.Installer {
	return chatgpt.NewInstaller(dataDir)
}

// paramPool hands out single-use captcha params minted ahead of demand. Because
// tokens cannot be replayed, this is the only way to keep a completion from
// paying the mint cost inline.
type paramPool struct {
	mu      sync.Mutex
	items   []captchaParam
	minting bool
}

// take pops a param that is still fresh.
func (p *paramPool) take() (captchaParam, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.items) > 0 {
		item := p.items[0]
		p.items = p.items[1:]
		if time.Since(item.MintedAt) < captchaMaxAge {
			return item, true
		}
	}
	return captchaParam{}, false
}

func (p *paramPool) put(item captchaParam) {
	if item.MintedAt.IsZero() {
		item.MintedAt = time.Now()
	}
	p.mu.Lock()
	p.items = append(p.items, item)
	p.mu.Unlock()
}

// reset drops every pooled param, used when the device identity is rotated.
func (p *paramPool) reset() {
	p.mu.Lock()
	p.items = nil
	p.mu.Unlock()
}

func (p *paramPool) size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.items)
}
