package chatgpt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	ProviderName = "chatgpt"
	Prefix       = "chatgpt/"
)

var CatalogModels = []string{
	"auto",
	"gpt-6",
	"gpt-6-mini",
	"gpt-5-6",
	"gpt-5-6-mini",
	"gpt-5-3-mini",
}

type CookieItem struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HttpOnly bool   `json:"httpOnly,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
}

type ProviderConfig struct {
	Enabled      bool   `json:"enabled"`
	BrowserPath  string `json:"browser_path,omitempty"`
	ShellEnabled bool   `json:"shell_enabled,omitempty"`
	WorkDir      string `json:"work_dir,omitempty"`
	// InternalTools opts out of handing tool calls to the calling agent.
	// Stored inverted so the zero value keeps passthrough enabled.
	InternalTools bool `json:"internal_tools,omitempty"`
}

type Config struct {
	BrowserPath     string
	CDPURL          string
	Headless        bool
	DataDir         string
	AutoIdleMinutes int
	WorkDir         string
	EnableShell     bool
}

type Client struct {
	cfg          Config
	installer    *Installer
	mu           sync.Mutex
	writeMu      sync.Mutex
	configMu     sync.RWMutex
	provConfig   ProviderConfig
	configFile   string
	cookiesFile  string
	cmd          *exec.Cmd
	wsConn       *websocket.Conn
	wsURL        string
	profileDir   string
	msgID        atomic.Int64
	subscribers  map[int64]chan streamToken
	pending      map[int64]chan cdpReply
	subMu        sync.Mutex
	readyMu      sync.Mutex
	ready        bool
	initWait     chan struct{}
	initErr      error
	lastUsed     time.Time
	readLoopStop chan struct{}
}

type streamToken struct {
	delta string
	done  bool
	err   error
}

// cdpReply is the correlated response to a CDP command.
type cdpReply struct {
	result json.RawMessage
	err    error
}

func NewClient(cfg Config) *Client {
	if cfg.DataDir == "" {
		cfg.DataDir = "data"
	}
	if abs, err := filepath.Abs(cfg.DataDir); err == nil {
		cfg.DataDir = abs
	}
	_ = os.MkdirAll(cfg.DataDir, 0755)

	installer := NewInstaller(cfg.DataDir)
	configFile := filepath.Join(cfg.DataDir, "chatgpt.json")
	cookiesFile := filepath.Join(cfg.DataDir, "cookies.json")

	c := &Client{
		cfg:         cfg,
		installer:   installer,
		configFile:  configFile,
		cookiesFile: cookiesFile,
		subscribers: make(map[int64]chan streamToken),
		pending:     make(map[int64]chan cdpReply),
		lastUsed:    time.Now(),
		provConfig: ProviderConfig{
			Enabled: false, // Disabled by default!
		},
	}

	// Load existing persisted config if present
	_ = c.loadConfig()

	if cfg.CDPURL == "" {
		c.cfg.CDPURL = os.Getenv("CHATGPT_CDP_URL")
	}
	if c.cfg.BrowserPath == "" {
		c.cfg.BrowserPath = installer.DetectedExecutable()
	}
	if c.cfg.BrowserPath != "" {
		c.provConfig.BrowserPath = c.cfg.BrowserPath
	}
	if cfg.AutoIdleMinutes <= 0 {
		c.cfg.AutoIdleMinutes = 15
	}
	if c.cfg.WorkDir == "" {
		if wd := os.Getenv("CHATGPT_WORKDIR"); wd != "" {
			c.cfg.WorkDir = wd
		} else if c.provConfig.WorkDir != "" {
			c.cfg.WorkDir = c.provConfig.WorkDir
		}
	}
	if !c.cfg.EnableShell {
		c.cfg.EnableShell = c.provConfig.ShellEnabled
	}
	return c
}

func (c *Client) loadConfig() error {
	data, err := os.ReadFile(c.configFile)
	if err != nil {
		return err
	}
	var pc ProviderConfig
	if err := json.Unmarshal(data, &pc); err != nil {
		return err
	}
	c.configMu.Lock()
	defer c.configMu.Unlock()
	c.provConfig = pc
	return nil
}

func (c *Client) saveConfigLocked() error {
	data, err := json.MarshalIndent(c.provConfig, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.configFile, data, 0644)
}

func (c *Client) IsEnabled() bool {
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	return c.provConfig.Enabled
}

func (c *Client) SetEnabled(enabled bool) error {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	if enabled {
		execPath := c.installer.DetectedExecutable()
		if execPath == "" && c.cfg.BrowserPath == "" && c.cfg.CDPURL == "" {
			return errors.New("browser not installed. Please download the browser first")
		}
		if c.cfg.BrowserPath == "" {
			c.cfg.BrowserPath = execPath
		}
		c.provConfig.Enabled = true
		c.provConfig.BrowserPath = c.cfg.BrowserPath
	} else {
		c.provConfig.Enabled = false
		go c.Close()
	}

	err := c.saveConfigLocked()
	if err == nil && enabled {
		// Warm the browser in the background so the first real request does not
		// pay cold-start cost and time out with a 502.
		go c.WarmUp()
	}
	return err
}

// WarmUp launches the browser and prepares the page ahead of the first request.
// Safe to call repeatedly; concurrent or already-warm calls return immediately.
func (c *Client) WarmUp() {
	if !c.IsEnabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := c.EnsureReady(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "[chatgpt] warm-up failed: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "[chatgpt] warm-up complete\n")
}

func (c *Client) HasCookies() bool {
	fi, err := os.Stat(c.cookiesFile)
	return err == nil && !fi.IsDir() && fi.Size() > 0
}

func (c *Client) SaveCookies(data []byte) error {
	// Validate JSON array of cookies
	var list []map[string]any
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("cookies must be a valid JSON array: %w", err)
	}

	if err := os.WriteFile(c.cookiesFile, data, 0600); err != nil {
		return fmt.Errorf("write cookies: %w", err)
	}

	// If browser is running, inject cookies immediately
	c.mu.Lock()
	conn := c.wsConn
	c.mu.Unlock()

	if conn != nil {
		go func() {
			_ = c.injectCookies(context.Background())
			_ = c.sendCDP("Page.navigate", map[string]any{"url": "https://chatgpt.com/"})
		}()
	}

	return nil
}

func (c *Client) ClearCookies() error {
	_ = os.Remove(c.cookiesFile)
	c.mu.Lock()
	conn := c.wsConn
	c.mu.Unlock()
	if conn != nil {
		go func() {
			_ = c.sendCDP("Network.clearBrowserCookies", nil)
			_ = c.sendCDP("Page.navigate", map[string]any{"url": "https://chatgpt.com/"})
		}()
	}
	return nil
}

func (c *Client) Installer() *Installer {
	return c.installer
}

func (c *Client) GetCatalogModels() []string {
	return CatalogModels
}

func IsCatalogModel(model string) bool {
	clean := CleanModelID(model)
	for _, m := range CatalogModels {
		if strings.EqualFold(m, clean) {
			return true
		}
	}
	return false
}

func HasPrefix(model string) bool {
	return strings.HasPrefix(model, Prefix)
}

func CleanModelID(model string) string {
	return strings.TrimPrefix(model, Prefix)
}

// EnsureReady ensures browser is running, CDP WebSocket is connected, and page has SentinelSDK primed.
func (c *Client) EnsureReady(ctx context.Context) error {
	if !c.IsEnabled() {
		return errors.New("ChatGPT Web provider is disabled")
	}

	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	// Readiness is tracked separately from the WebSocket: the connection is
	// established before the page is navigated and the driver injected, so a
	// request arriving mid-initialisation must wait instead of racing ahead and
	// failing with an undefined a2StreamTurn. A ready flag without a live socket
	// is stale (browser exited), so both are required.
	c.readyMu.Lock()
	c.mu.Lock()
	live := c.wsConn != nil
	c.mu.Unlock()
	if c.ready && !live {
		c.ready = false
	}
	if c.ready {
		c.readyMu.Unlock()
		return nil
	}
	if wait := c.initWait; wait != nil {
		c.readyMu.Unlock()
		select {
		case <-wait:
			c.readyMu.Lock()
			ready, initErr := c.ready, c.initErr
			c.readyMu.Unlock()
			if ready {
				return nil
			}
			if initErr != nil {
				return fmt.Errorf("chatgpt browser initialization failed: %w", initErr)
			}
			return errors.New("chatgpt browser initialization failed")
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	done := make(chan struct{})
	c.initWait = done
	c.readyMu.Unlock()

	// Initialisation runs under its own deadline rather than the caller's
	// context. Otherwise a cancelled caller (a timed-out warm-up, a disconnected
	// HTTP client) would abort an initialisation that other requests are waiting
	// on, and they would all observe a failure.
	initCtx, cancelInit := context.WithTimeout(context.Background(), initTimeout)

	initDone := make(chan error, 1)
	go func() { initDone <- c.doInit(initCtx) }()

	// A single finaliser owns the bookkeeping and publishes it before waking any
	// waiter, so waiters never observe a ready flag that has not been set yet.
	finalised := make(chan struct{})
	go func() {
		defer close(finalised)
		err := <-initDone
		cancelInit()

		c.readyMu.Lock()
		if err == nil {
			c.ready = true
		}
		c.initErr = err
		c.initWait = nil
		close(done)
		c.readyMu.Unlock()

		if err != nil {
			// Drop the half-initialised connection so the next attempt
			// reconnects instead of reusing a page that never became usable.
			c.teardownConnection()
		}
	}()

	select {
	case <-finalised:
	case <-ctx.Done():
		// Stop waiting, but leave initialisation running for the next caller.
		return ctx.Err()
	}

	c.readyMu.Lock()
	ready, initErr := c.ready, c.initErr
	c.readyMu.Unlock()
	if ready {
		return nil
	}
	if initErr != nil {
		return initErr
	}
	return errors.New("chatgpt browser initialization failed")
}

// initTimeout bounds a full browser launch plus page preparation.
const initTimeout = 180 * time.Second

// relaunchTimeout bounds the fresh-browser retry used when an attached browser
// turned out to be a stale endpoint.
const relaunchTimeout = 120 * time.Second

// doInit establishes the CDP connection (attaching or launching) and prepares
// the page. It runs at most once at a time, guarded by initWait.
func (c *Client) doInit(ctx context.Context) error {
	c.mu.Lock()
	haveConn := c.wsConn != nil
	c.mu.Unlock()

	if !haveConn {
		attached := false
		if c.cfg.CDPURL != "" {
			if err := c.connectCDP(ctx, c.cfg.CDPURL); err != nil {
				if lerr := c.launchBrowser(ctx); lerr != nil {
					return fmt.Errorf("chatgpt launch browser: %w", lerr)
				}
			} else {
				attached = true
			}
		} else if err := c.attachExisting(ctx); err == nil {
			// A browser from a previous run can still be listening; reuse it
			// instead of racing a second Chromium for the same debug port.
			attached = true
			if debugEnabled() {
				fmt.Fprintln(os.Stderr, "[chatgpt] attached to an existing browser on the debug port")
			}
		} else {
			if err := c.launchBrowser(ctx); err != nil {
				return fmt.Errorf("chatgpt launch browser: %w", err)
			}
		}

		err := c.initPage(ctx)
		if err == nil || !attached {
			return err
		}

		// The endpoint answered but the page never became usable: a leftover
		// Chromium helper was still holding the port. Drop it and start clean.
		// This gets its own budget because the failed attach has usually already
		// consumed most of the initialisation deadline.
		if debugEnabled() {
			fmt.Fprintf(os.Stderr, "[chatgpt] attached browser unusable (%v); launching a fresh one\n", err)
		}
		c.teardownConnection()
		c.mu.Lock()
		c.killBrowserLocked()
		c.mu.Unlock()

		relaunchCtx, cancelRelaunch := context.WithTimeout(context.Background(), relaunchTimeout)
		defer cancelRelaunch()
		if lerr := c.launchBrowser(relaunchCtx); lerr != nil {
			return fmt.Errorf("chatgpt launch browser after stale endpoint: %w", lerr)
		}
		return c.initPage(relaunchCtx)
	}

	return c.initPage(ctx)
}

// cdpTarget is one entry from the DevTools /json/list endpoint.
type cdpTarget struct {
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// pickPageTarget chooses the target to drive. Workers and iframes (the sentinel
// runner, for instance) also expose a WebSocket debugger URL, and attaching to
// one of those silently breaks navigation, so a page target is required.
func pickPageTarget(tabs []cdpTarget) string {
	best := ""
	for _, t := range tabs {
		if t.WebSocketDebuggerURL == "" || t.Type != "page" {
			continue
		}
		if strings.Contains(t.URL, "chatgpt.com") {
			return t.WebSocketDebuggerURL
		}
		if best == "" {
			best = t.WebSocketDebuggerURL
		}
	}
	return best
}

// attachExisting connects to a Chrome DevTools endpoint already listening on the
// configured debug port, if one is present and healthy.
func (c *Client) attachExisting(ctx context.Context) error {
	port := c.debugPort()
	endpoint := fmt.Sprintf("http://127.0.0.1:%s/json/list", port)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		return fmt.Errorf("no existing cdp endpoint on port %s", port)
	}
	var tabs []cdpTarget
	_ = json.NewDecoder(resp.Body).Decode(&tabs)
	resp.Body.Close()

	wsURL := pickPageTarget(tabs)
	if wsURL == "" {
		return fmt.Errorf("existing cdp endpoint has no attachable page target")
	}
	return c.connectCDP(ctx, wsURL)
}

// debugPort resolves the remote debugging port to use.
func (c *Client) debugPort() string {
	if c.cfg.CDPURL != "" {
		parts := strings.Split(c.cfg.CDPURL, ":")
		if len(parts) >= 3 {
			return strings.TrimPrefix(parts[2], "/")
		}
	}
	return "9222"
}

// killBrowserLocked terminates the whole browser process tree. Chromium helpers
// survive a plain kill of the main process and keep the debug port bound, which
// makes the next launch attach to a zombie endpoint. Callers must hold c.mu.
func (c *Client) killBrowserLocked() {
	if c.cmd != nil && c.cmd.Process != nil {
		pid := c.cmd.Process.Pid
		if err := killProcessGroup(pid); err != nil {
			_ = c.cmd.Process.Kill()
		}
		_ = c.cmd.Wait()
	}
	c.cmd = nil
	if c.profileDir != "" {
		_ = os.RemoveAll(c.profileDir)
		c.profileDir = ""
	}
}

// markConnectionLost invalidates readiness after the CDP socket drops, so the
// next request reconnects and relaunches the browser if it has exited.
func (c *Client) markConnectionLost(conn *websocket.Conn) {
	c.mu.Lock()
	if c.wsConn != conn {
		// A newer connection replaced this one; leave it alone.
		c.mu.Unlock()
		return
	}
	c.wsConn = nil
	if c.readLoopStop != nil {
		close(c.readLoopStop)
		c.readLoopStop = nil
	}
	if c.cmd != nil && c.cmd.Process != nil && !processAlive(c.cmd.Process.Pid) {
		c.killBrowserLocked()
	}
	c.mu.Unlock()

	c.readyMu.Lock()
	c.ready = false
	c.readyMu.Unlock()

	if debugEnabled() {
		fmt.Fprintln(os.Stderr, "[chatgpt] cdp connection lost; provider marked not ready")
	}
}

// processAlive reports whether a pid is still running.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// teardownConnection clears the socket and ready flag without stopping the browser.
func (c *Client) teardownConnection() {
	c.mu.Lock()
	if c.readLoopStop != nil {
		close(c.readLoopStop)
		c.readLoopStop = nil
	}
	if c.wsConn != nil {
		_ = c.wsConn.Close()
		c.wsConn = nil
	}
	c.mu.Unlock()

	c.readyMu.Lock()
	c.ready = false
	c.readyMu.Unlock()
}

func (c *Client) launchBrowser(ctx context.Context) error {
	browserPath := c.cfg.BrowserPath
	if browserPath == "" {
		browserPath = c.installer.DetectedExecutable()
	}
	if browserPath == "" {
		return errors.New("no browser executable found. Please install the browser first")
	}

	port := c.debugPort()
	port = strings.TrimSpace(port)
	if port == "" {
		port = "9222"
	}

	c.profileDir = filepath.Join(os.TempDir(), "lun_chatgpt_profile")
	_ = os.MkdirAll(c.profileDir, 0700)

	args := []string{
		"--headless=new",
		fmt.Sprintf("--remote-debugging-port=%s", port),
		"--remote-allow-origins=*",
		fmt.Sprintf("--user-data-dir=%s", c.profileDir),
		"--renderer-process-limit=1",
		"--in-process-gpu",
		"--window-size=800,600",
		"--mute-audio",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-breakpad",
		"--disable-component-extensions-with-background-pages",
		"--disable-component-update",
		"--disable-extensions",
		"--disable-features=Translate,OptimizationHints,MediaRouter,DialMediaRouteProvider,CalculateNativeWinOcclusion,InterestFeedContentSuggestions,CertificateTransparencyComponentUpdater",
		"--disable-ipc-flooding-protection",
		"--disable-renderer-backgrounding",
		"--disable-search-engine-choice-screen",
		"--disable-sync",
		"--metrics-recording-only",
		"--js-flags=--max-old-space-size=256",
		"about:blank",
	}

	cmd := exec.Command(browserPath, args...)
	// Own process group so the whole Chromium tree can be reaped as a unit;
	// orphaned helpers keep the debug port bound and poison the next launch.
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("exec %s: %w", browserPath, err)
	}
	c.cmd = cmd

	endpoint := fmt.Sprintf("http://127.0.0.1:%s", port)
	var wsURL string
	start := time.Now()
	for time.Since(start) < 15*time.Second {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}

		resp, err := http.Get(endpoint + "/json/list")
		if err == nil && resp.StatusCode == 200 {
			var tabs []cdpTarget
			_ = json.NewDecoder(resp.Body).Decode(&tabs)
			resp.Body.Close()
			wsURL = pickPageTarget(tabs)
			if wsURL != "" {
				break
			}
		}
	}

	if wsURL == "" {
		_ = cmd.Process.Kill()
		return errors.New("timed out waiting for Chromium page to appear on CDP")
	}

	time.Sleep(1 * time.Second)
	return c.connectCDP(ctx, wsURL)
}

func (c *Client) connectCDP(ctx context.Context, wsURL string) error {
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("cdp dial %s: %w", wsURL, err)
	}

	c.wsConn = conn
	c.wsURL = wsURL
	c.readLoopStop = make(chan struct{})

	go c.readLoop(conn, c.readLoopStop)
	return nil
}

func (c *Client) readLoop(conn *websocket.Conn, stop chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			// The socket died, which almost always means the browser exited.
			// Readiness must be invalidated or the next request would skip
			// initialisation and fail with "cdp not connected" forever.
			c.markConnectionLost(conn)
			return
		}

		var event struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			continue
		}

		// Correlation reply for a command we sent.
		if event.ID != nil {
			c.mu.Lock()
			waiter, ok := c.pending[*event.ID]
			if ok {
				delete(c.pending, *event.ID)
			}
			c.mu.Unlock()
			if ok {
				if event.Error != nil {
					waiter <- cdpReply{err: errors.New(event.Error.Message)}
				} else {
					waiter <- cdpReply{result: event.Result}
				}
			}
			continue
		}

		if event.Method != "Runtime.bindingCalled" || len(event.Params) == 0 {
			if debugEnabled() && event.Method != "" {
				fmt.Fprintf(os.Stderr, "[chatgpt-cdp] event %s\n", event.Method)
			}
			continue
		}
		var payload struct {
			Name    string `json:"name"`
			Payload string `json:"payload"`
		}
		if err := json.Unmarshal(event.Params, &payload); err != nil {
			continue
		}
		if payload.Name != "onToken" {
			if debugEnabled() {
				fmt.Fprintf(os.Stderr, "[chatgpt-cdp] bindingCalled %s payload=%s\n", payload.Name, truncateForLog(payload.Payload))
			}
			continue
		}
		var p struct {
			ReqID int64  `json:"reqId"`
			Delta string `json:"delta"`
			Done  bool   `json:"done"`
			Err   string `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload.Payload), &p); err != nil {
			continue
		}
		c.subMu.Lock()
		ch, ok := c.subscribers[p.ReqID]
		c.subMu.Unlock()
		if !ok {
			continue
		}
		if p.Err != "" {
			ch <- streamToken{err: errors.New(p.Err)}
		} else if p.Done {
			ch <- streamToken{done: true}
		} else {
			ch <- streamToken{delta: p.Delta}
		}
	}
}

func (c *Client) sendCDP(method string, params any) error {
	c.mu.Lock()
	conn := c.wsConn
	c.mu.Unlock()
	if conn == nil {
		return errors.New("cdp not connected")
	}

	id := c.msgID.Add(1)
	payload := map[string]any{"id": id, "method": method}
	if params != nil {
		payload["params"] = params
	}
	data, _ := json.Marshal(payload)

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, data)
}

// callCDP sends a command and waits for its correlated reply.
func (c *Client) callCDP(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	conn := c.wsConn
	if conn == nil {
		c.mu.Unlock()
		return nil, errors.New("cdp not connected")
	}
	id := c.msgID.Add(1)
	waiter := make(chan cdpReply, 1)
	c.pending[id] = waiter
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	payload := map[string]any{"id": id, "method": method}
	if params != nil {
		payload["params"] = params
	}
	data, _ := json.Marshal(payload)

	c.writeMu.Lock()
	err := conn.WriteMessage(websocket.TextMessage, data)
	c.writeMu.Unlock()
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case reply := <-waiter:
		if reply.err != nil {
			return nil, reply.err
		}
		return reply.result, nil
	}
}

// evaluateStringOnPage evaluates an async expression in the page's main frame
// and returns its string value.
func (c *Client) evaluateStringOnPage(expression string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	return c.evaluateStringOnPageCtx(ctx, expression)
}

// evaluateStringOnPageCtx is evaluateStringOnPage with a caller-supplied context.
func (c *Client) evaluateStringOnPageCtx(ctx context.Context, expression string) (string, error) {
	raw, err := c.callCDP(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"awaitPromise":  true,
		"returnByValue": true,
	})
	if err != nil {
		return "", err
	}
	if ex := cdpException(raw); ex != "" {
		return "", fmt.Errorf("page exception: %s", ex)
	}
	var res struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.Result.Type != "string" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(res.Result.Value, &s); err != nil {
		return "", err
	}
	return s, nil
}

func (c *Client) injectCookies(ctx context.Context) error {
	data, err := os.ReadFile(c.cookiesFile)
	if err != nil {
		return nil // no cookies file, proceed anonymously
	}

	var rawList []map[string]any
	if err := json.Unmarshal(data, &rawList); err != nil {
		return err
	}

	var cdpCookies []map[string]any
	for _, c := range rawList {
		name, _ := c["name"].(string)
		val, _ := c["value"].(string)
		dom, _ := c["domain"].(string)
		if name == "" || val == "" || dom == "" {
			continue
		}
		cookie := map[string]any{
			"name":   name,
			"value":  val,
			"domain": dom,
			"path":   "/",
		}
		if p, ok := c["path"].(string); ok && p != "" {
			cookie["path"] = p
		}
		if s, ok := c["secure"].(bool); ok {
			cookie["secure"] = s
		}
		if h, ok := c["httpOnly"].(bool); ok {
			cookie["httpOnly"] = h
		}
		if ss, ok := c["sameSite"].(string); ok && ss != "" {
			switch strings.ToLower(ss) {
			case "strict":
				cookie["sameSite"] = "Strict"
			case "lax":
				cookie["sameSite"] = "Lax"
			case "none", "no_restriction":
				cookie["sameSite"] = "None"
			}
		}
		cdpCookies = append(cdpCookies, cookie)
	}

	if len(cdpCookies) > 0 {
		_ = c.sendCDP("Network.setCookies", map[string]any{"cookies": cdpCookies})
	}
	return nil
}

func (c *Client) initPage(ctx context.Context) error {
	_ = c.sendCDP("Network.enable", nil)

	// Inject cookies BEFORE navigating to chatgpt.com
	_ = c.injectCookies(ctx)

	// Navigate to ChatGPT
	_ = c.sendCDP("Page.navigate", map[string]any{"url": "https://chatgpt.com/"})

	// Wait for the document to actually finish loading before touching its DOM.
	// Injecting the sentinel SDK into a still-compiling page silently fails and
	// turns the first request into a 502.
	if err := c.waitForDocument(ctx); err != nil {
		return err
	}

	// Enable Runtime and add binding AFTER navigation so it binds to the new document execution context
	_ = c.sendCDP("Runtime.enable", nil)
	_ = c.sendCDP("Runtime.addBinding", map[string]string{"name": "onToken"})

	// Ensure the sentinel SDK is present and usable before reporting ready.
	if err := c.ensureSentinelReady(ctx); err != nil {
		return err
	}

	// Inject A2 pure-fetch driver with ensureSentinel guard
	driverScript := `
async function ensureSentinel() {
  if (window.SentinelSDK) return;
  await new Promise((res, rej) => {
    const s = document.createElement('script');
    s.src = '/backend-api/sentinel/sdk.js';
    s.onload = () => setTimeout(res, 500);
    s.onerror = () => rej(new Error('Failed to load sentinel sdk.js'));
    document.head.appendChild(s);
  });
}

async function a2StreamTurn(promptText, reqId) {
  try {
    await ensureSentinel();
    const raw = await window.SentinelSDK.token("conversation");
    const tokObj = typeof raw === "string" ? JSON.parse(raw) : raw;

    const opId = crypto.randomUUID();
    const uId = crypto.randomUUID();
    const aId = "pending-" + crypto.randomUUID();
    const sessId = (document.cookie.match(/oai-did=([^;]+)/) || [])[1] || crypto.randomUUID();

    const params = new URLSearchParams();
    params.set("prompt", promptText);
    params.set("userMessageId", uId);
    params.set("assistantMessageId", aId);
    params.set("conversationState", JSON.stringify({messages:[],parentMessageId:"client-created-root",safety:{dismissedInterventionIds:[]},userMessageCount:0}));
    params.set("oai-session-id", sessId);
    params.set("turnstileToken", tokObj.t || "");
    params.set("proofToken", tokObj.p || "");
    params.set("chatRequirementsToken", tokObj.c || "");
    params.set("timezone", "Asia/Jakarta");
    params.set("timezoneOffsetMinutes", "-420");

    const resp = await fetch("/unauth-mweb/conversation/updates?lightweight_authenticated=0&operationId=" + opId, {
      method: "POST",
      headers: {
        "Accept": "application/vnd.openai.conversation-source+ndjson",
        "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
        "X-Web-Mobile-Conversation-Renderer": "octane",
        "X-Web-Mobile-Conversation-Stream-Protocol": "2",
        "X-Web-Mobile-Conversation-Source": "1",
        "OAI-Session-Id": sessId
      },
      body: params.toString()
    });

    if (!resp.ok) {
      window.onToken(JSON.stringify({ reqId, error: "HTTP " + resp.status + ": " + (await resp.text()).slice(0, 200) }));
      return;
    }

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    // Track each assistant message independently. The stream interleaves
    // revisions and replace/append modes; a single global counter made the
    // deltas misalign and duplicated text in the reply.
    const messages = new Map();

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      const chunk = decoder.decode(value, { stream: true });
      for (const line of chunk.split("\n")) {
        if (!line.trim()) continue;
        try {
          const obj = JSON.parse(line);
          if (obj.type !== "content" || !obj.markdown) continue;

          const id = obj.messageId || "default";
          const state = messages.get(id) || { text: "", revision: -1 };
          const revision = typeof obj.revision === "number" ? obj.revision : state.revision + 1;
          if (revision <= state.revision) continue;

          let delta = "";
          if (obj.mode === "append") {
            delta = obj.markdown;
            state.text += obj.markdown;
          } else {
            // replace: markdown is the authoritative text so far. Emit only what
            // changed by diffing against the previous text.
            let i = 0;
            while (i < state.text.length && i < obj.markdown.length &&
                   state.text.charCodeAt(i) === obj.markdown.charCodeAt(i)) i++;
            delta = obj.markdown.slice(i);
            state.text = obj.markdown;
          }
          state.revision = revision;
          messages.set(id, state);

          if (delta) window.onToken(JSON.stringify({ reqId, delta }));
        } catch (err) {}
      }
    }
    window.onToken(JSON.stringify({ reqId, done: true }));
  } catch (err) {
    window.onToken(JSON.stringify({ reqId, error: String(err) + " @ page=" + location.href + " ready=" + document.readyState }));
  }
}`
	_ = c.sendCDP("Runtime.evaluate", map[string]any{
		"expression": driverScript,
	})

	return nil
}

// runTurn sends one prompt to the ChatGPT web page and collects the full reply text.
// onDelta, when non-nil, receives incremental deltas as they stream in.
func (c *Client) runTurn(ctx context.Context, prompt string, onDelta func(string)) (string, error) {
	reqID := c.msgID.Add(1)
	tokenCh := make(chan streamToken, 256)

	c.subMu.Lock()
	c.subscribers[reqID] = tokenCh
	c.subMu.Unlock()

	defer func() {
		c.subMu.Lock()
		delete(c.subscribers, reqID)
		c.subMu.Unlock()
	}()

	// The page is a long-lived SPA: a client-side reload wipes window.onToken and
	// the injected driver, after which a silent exception would hang the request
	// until timeout. Verify the driver and fail fast instead.
	if err := c.ensureDriver(ctx); err != nil {
		return "", err
	}

	expr := fmt.Sprintf(`a2StreamTurn(%s, %d)`, jsonEscape(prompt), reqID)
	dispatchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	raw, err := c.callCDP(dispatchCtx, "Runtime.evaluate", map[string]any{
		"expression":   expr,
		"awaitPromise": false,
	})
	if err != nil {
		return "", fmt.Errorf("send turn evaluate: %w", err)
	}
	if ex := cdpException(raw); ex != "" {
		return "", fmt.Errorf("turn dispatch failed: %s", ex)
	}

	var full strings.Builder
	for {
		select {
		case <-ctx.Done():
			return full.String(), ctx.Err()
		case t, ok := <-tokenCh:
			if !ok {
				return full.String(), nil
			}
			if t.err != nil {
				return full.String(), t.err
			}
			if t.done {
				return full.String(), nil
			}
			full.WriteString(t.delta)
			if onDelta != nil && t.delta != "" {
				onDelta(t.delta)
			}
		}
	}
}

// ExecuteChat dispatches a prompt and yields streaming tokens or blocks for completion.
func (c *Client) ExecuteChat(ctx context.Context, body map[string]any, stream bool) (*http.Response, error) {
	if !c.IsEnabled() {
		return nil, errors.New("ChatGPT Web provider is disabled")
	}

	if err := c.EnsureReady(ctx); err != nil {
		return nil, err
	}

	specs := ParseToolSpecs(body)
	prompt := extractPrompt(body, len(specs) > 0)
	if prompt == "" {
		return nil, errors.New("empty prompt in request messages")
	}

	model, _ := body["model"].(string)
	if model == "" {
		model = "auto"
	}
	reqID := c.msgID.Load()

	// Tool-enabled requests ask the model for a tool call. When it produces one we
	// hand it back to the client in OpenAI format so the agent UI can display and
	// execute it (Pi, OpenCode and friends own real sandboxed tools). Only when the
	// model refuses do we fall back to executing the tool inside the router.
	if len(specs) > 0 {
		answer, passthroughCall, err := c.runToolLoop(ctx, prompt, body, specs, c.PassthroughTools())
		if err != nil {
			return nil, err
		}
		if passthroughCall != nil {
			return buildToolCallResponse(passthroughCall, model, reqID, stream), nil
		}
		return buildChatResponse(answer, model, reqID, stream), nil
	}

	if !stream {
		text, err := c.runTurnWithRecovery(ctx, prompt, nil)
		if err != nil {
			return nil, err
		}
		return buildChatResponse(text, model, reqID, false), nil
	}

	// Streaming passthrough for plain chat requests.
	pr, pw := io.Pipe()
	deltaCh := make(chan string, 256)
	done := make(chan struct{})
	var turnErr error

	go func() {
		defer close(done)
		_, turnErr = c.runTurnWithRecovery(ctx, prompt, func(d string) {
			select {
			case deltaCh <- d:
			case <-ctx.Done():
			}
		})
		close(deltaCh)
	}()

	go func() {
		defer pw.Close()
		for d := range deltaCh {
			chunk := map[string]any{
				"id":      fmt.Sprintf("chatcmpl-%d", reqID),
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   model,
				"choices": []map[string]any{
					{"index": 0, "delta": map[string]string{"content": d}},
				},
			}
			b, _ := json.Marshal(chunk)
			_, _ = fmt.Fprintf(pw, "data: %s\n\n", b)
		}
		<-done
		if turnErr != nil {
			_, _ = fmt.Fprintf(pw, "data: %s\n\n", mustJSON(map[string]any{
				"error": map[string]string{"message": turnErr.Error(), "type": "api_error"},
			}))
		} else {
			finalChunk := map[string]any{
				"id":      fmt.Sprintf("chatcmpl-%d", reqID),
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   model,
				"choices": []map[string]any{
					{"index": 0, "delta": map[string]string{}, "finish_reason": "stop"},
				},
			}
			_, _ = fmt.Fprintf(pw, "data: %s\n\n", mustJSON(finalChunk))
			_, _ = fmt.Fprint(pw, "data: [DONE]\n\n")
		}
	}()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       pr,
	}
	resp.Header.Set("Content-Type", "text/event-stream")
	resp.Header.Set("Cache-Control", "no-cache")
	resp.Header.Set("Connection", "keep-alive")
	return resp, nil
}

// isTransientUpstream reports whether an upstream error is worth one retry on a
// freshly reloaded page (rate limiting, bot scoring, transient 5xx).
func isTransientUpstream(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{"403", "429", "unusual activity", "too many requests", "502", "503", "504", "network error", "failed to fetch", "load failed", "networkerror", "deadline exceeded", "context canceled"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// runTurnWithRecovery runs one turn with a hard timeout and a single reload-and-
// retry when the upstream rejects the request transiently. Without this a
// stochastic 403 reaches the client as a 502.
func (c *Client) runTurnWithRecovery(ctx context.Context, prompt string, onDelta func(string)) (string, error) {
	turnCtx, cancel := context.WithTimeout(ctx, turnTimeout)
	defer cancel()

	// Once any delta has reached the client a retry would duplicate the visible
	// reply, so streaming turns are not retried after partial output.
	emitted := false
	forward := onDelta
	if onDelta != nil {
		forward = func(d string) {
			emitted = true
			onDelta(d)
		}
	}

	text, err := c.runTurn(turnCtx, prompt, forward)
	if err == nil || text != "" {
		return text, err
	}
	if emitted {
		return text, err
	}
	if !isTransientUpstream(err) {
		return text, err
	}

	if debugEnabled() {
		fmt.Fprintf(os.Stderr, "[chatgpt] transient upstream error, reloading page and retrying: %v\n", err)
	}

	select {
	case <-ctx.Done():
		return text, err
	case <-time.After(2 * time.Second):
	}

	// A stale page state is the usual cause, so re-prepare the page first.
	if rerr := c.initPage(ctx); rerr != nil {
		if debugEnabled() {
			fmt.Fprintf(os.Stderr, "[chatgpt] page re-init failed: %v\n", rerr)
		}
		return text, err
	}

	retryCtx, cancel2 := context.WithTimeout(ctx, turnTimeout)
	defer cancel2()
	retryText, retryErr := c.runTurn(retryCtx, prompt, forward)
	if retryErr != nil && retryText == "" {
		// Report the original error if the retry also fails at the same point.
		if debugEnabled() {
			fmt.Fprintf(os.Stderr, "[chatgpt] retry also failed: %v\n", retryErr)
		}
		return retryText, retryErr
	}
	return retryText, retryErr
}

// turnTimeout bounds a single upstream turn so a stuck page cannot hang a request.
const turnTimeout = 100 * time.Second

// cdpException extracts the exception text from a Runtime.evaluate reply.
func cdpException(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var res struct {
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception *struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return ""
	}
	if res.ExceptionDetails == nil {
		return ""
	}
	if res.ExceptionDetails.Exception != nil && res.ExceptionDetails.Exception.Description != "" {
		return res.ExceptionDetails.Exception.Description
	}
	return res.ExceptionDetails.Text
}

// driverProbe reports whether the injected turn driver and CDP binding are both
// still present in the current document.
const driverProbe = `(function(){
	return (typeof a2StreamTurn === 'function'
		&& typeof window.onToken === 'function'
		&& !!window.SentinelSDK) ? 'ok' : 'missing';
})()`

// ensureDriver re-prepares the page when a client-side reload has discarded the
// driver, instead of letting the turn hang until the request times out.
func (c *Client) ensureDriver(ctx context.Context) error {
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	val, err := c.evaluateStringOnPageCtx(probeCtx, driverProbe)
	if err == nil && val == "ok" {
		return nil
	}
	if debugEnabled() {
		fmt.Fprintf(os.Stderr, "[chatgpt] driver missing (err=%v val=%q); re-preparing page\n", err, val)
	}
	if err != nil {
		return fmt.Errorf("page probe failed: %w", err)
	}
	return c.initPage(ctx)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// callIDCounter produces unique OpenAI-style tool call ids.
var callIDCounter atomic.Int64

func newToolCallID() string {
	return fmt.Sprintf("call_%d_%d", time.Now().UnixNano()%1e9, callIDCounter.Add(1))
}

// buildToolCallResponse returns an OpenAI chat completion carrying tool_calls so
// the calling agent can display the step and run the tool with its own
// sandboxed implementation. The assistant turn ends with finish_reason
// "tool_calls"; the client then replies with role "tool" results and the router
// continues the conversation on the next request.
func buildToolCallResponse(call *ToolCall, model string, reqID int64, stream bool) *http.Response {
	toolCall := map[string]any{
		"id":   newToolCallID(),
		"type": "function",
		"function": map[string]any{
			"name":      call.Name,
			"arguments": mustJSON(call.Arguments),
		},
	}

	if !stream {
		respObj := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-%d", reqID),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{
					"index":         0,
					"message":       map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{toolCall}},
					"finish_reason": "tool_calls",
				},
			},
		}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(mustJSON(respObj))),
		}
		resp.Header.Set("Content-Type", "application/json")
		return resp
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		roleChunk := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-%d", reqID),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{"index": 0, "delta": map[string]any{"role": "assistant", "content": nil}},
			},
		}
		_, _ = fmt.Fprintf(pw, "data: %s\n\n", mustJSON(roleChunk))

		tc := map[string]any{
			"index": 0,
			"id":    toolCall["id"],
			"type":  "function",
			"function": map[string]any{
				"name":      call.Name,
				"arguments": mustJSON(call.Arguments),
			},
		}
		callChunk := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-%d", reqID),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{"index": 0, "delta": map[string]any{"tool_calls": []any{tc}}},
			},
		}
		_, _ = fmt.Fprintf(pw, "data: %s\n\n", mustJSON(callChunk))

		finalChunk := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-%d", reqID),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"},
			},
		}
		_, _ = fmt.Fprintf(pw, "data: %s\n\n", mustJSON(finalChunk))
		_, _ = fmt.Fprint(pw, "data: [DONE]\n\n")
	}()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       pr,
	}
	resp.Header.Set("Content-Type", "text/event-stream")
	resp.Header.Set("Cache-Control", "no-cache")
	resp.Header.Set("Connection", "keep-alive")
	return resp
}

func debugEnabled() bool {
	return os.Getenv("CHATGPT_DEBUG_BODY") == "1"
}

func truncateForLog(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

func buildChatResponse(text, model string, reqID int64, stream bool) *http.Response {
	if !stream {
		respObj := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-%d", reqID),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{
					"index":         0,
					"message":       map[string]string{"role": "assistant", "content": text},
					"finish_reason": "stop",
				},
			},
		}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(mustJSON(respObj))),
		}
		resp.Header.Set("Content-Type", "application/json")
		return resp
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		chunk := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-%d", reqID),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{"index": 0, "delta": map[string]string{"content": text}},
			},
		}
		_, _ = fmt.Fprintf(pw, "data: %s\n\n", mustJSON(chunk))
		finalChunk := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-%d", reqID),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{"index": 0, "delta": map[string]string{}, "finish_reason": "stop"},
			},
		}
		_, _ = fmt.Fprintf(pw, "data: %s\n\n", mustJSON(finalChunk))
		_, _ = fmt.Fprint(pw, "data: [DONE]\n\n")
	}()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       pr,
	}
	resp.Header.Set("Content-Type", "text/event-stream")
	resp.Header.Set("Cache-Control", "no-cache")
	resp.Header.Set("Connection", "keep-alive")
	return resp
}

// maxToolRounds bounds the internal agent loop.
const maxToolRounds = 8

// runToolLoop drives a bounded ReAct-style loop: the model asks for a tool, the
// host executes it, the transcript is fed back, until the model answers in prose.
//
// Relative paths are resolved against an auto-detected project directory rather
// than requiring the operator to configure one: the request body and prompt are
// scanned for a cwd/workspace hint, then for absolute paths whose project root
// can be climbed to from disk.
func (c *Client) runToolLoop(ctx context.Context, prompt string, body map[string]any, specs []ToolSpec, passthrough bool) (string, *ToolCall, error) {
	bases := c.CandidateWorkDirs(prompt, body)
	executed := ExecutedToolCalls(body)
	workdirHint := ""
	if len(bases) > 0 {
		workdirHint = fmt.Sprintf("\n\n[RESOLVED WORKSPACE]\nRelative paths resolve against: %s\n", bases[0])
	}

	transcript := toolPreamble(specs) + workdirHint + prompt
	usedShellFallback := false
	didReadFallback := false
	forcedToolRound := false
	if debugEnabled() {
		fmt.Fprintf(os.Stderr, "[chatgpt-tool] work dir candidates: %v, passthrough=%v\n", bases, passthrough)
	}

	if debugEnabled() {
		names := make([]string, 0, len(specs))
		for _, s := range specs {
			names = append(names, s.Name)
		}
		fmt.Fprintf(os.Stderr, "[chatgpt-tool] parsed tools: %v, executed: %d\n", names, len(executed))
	}

	for round := 0; round < maxToolRounds; round++ {
		reply, err := c.runTurnWithRecovery(ctx, transcript, nil)
		if err != nil {
			if reply != "" {
				return reply, nil, nil
			}
			return "", nil, err
		}

		call, clean := ExtractToolCall(reply)
		if call != nil {
			call = NormalizeToolCall(specs, call)
		}
		if debugEnabled() {
			fmt.Fprintf(os.Stderr, "[chatgpt-tool] round=%d reply=%q parsed_call=%v\n", round+1, truncateForLog(reply), call != nil)
		}
		if call != nil && passthrough && round == 0 && hasToolNamed(specs, call.Name) && !callAlreadyMade(executed, call) {
			// Hand the call to the client so its UI shows the step and its own
			// sandboxed tool implementation performs the work.
			if debugEnabled() {
				fmt.Fprintf(os.Stderr, "[chatgpt-tool] passing tool call through to client: %s %s\n", call.Name, mustJSON(call.Arguments))
			}
			return "", call, nil
		}
		if call != nil && callAlreadyMade(executed, call) {
			// The agent already ran this; the model is repeating itself. Continue
			// the turn instead of bouncing the same call back forever.
			if debugEnabled() {
				fmt.Fprintf(os.Stderr, "[chatgpt-tool] suppressing repeated tool call %s %s\n", call.Name, mustJSON(call.Arguments))
			}
			call = nil
		}
		if call == nil && passthrough && round == 0 {
			// The model declined to emit a tool call. When the request implies one,
			// synthesise it and hand that to the client so the agent still shows the
			// step and does the work.
			if inferred := InferBuildCall(prompt, reply, specs, bases, executed); inferred != nil {
				if debugEnabled() {
					fmt.Fprintf(os.Stderr, "[chatgpt-tool] inferred tool call for client: %s %s\n", inferred.Name, truncateForLog(mustJSON(inferred.Arguments)))
				}
				return "", inferred, nil
			}
			// Nothing could be inferred safely. Re-ask once, explicitly, because an
			// agent client terminates its loop on a reply with no tool call.
			if !forcedToolRound && wantsToolUse(prompt, reply) {
				forcedToolRound = true
				if debugEnabled() {
					fmt.Fprintln(os.Stderr, "[chatgpt-tool] no tool call; issuing forced tool round")
				}
				transcript += "\n\nAssistant: " + reply + forcedToolDirective
				continue
			}
			// The model refused even the explicit re-ask. Rather than let the agent
			// terminate, hand it one safe call against a path that exists on disk.
		}
		if call == nil && passthrough && forcedToolRound {
			if progress := InferProgressCall(prompt, specs, bases); progress != nil && !callAlreadyMade(executed, progress) {
				if debugEnabled() {
					fmt.Fprintf(os.Stderr, "[chatgpt-tool] progress fallback for client: %s %s\n", progress.Name, truncateForLog(mustJSON(progress.Arguments)))
				}
				return "", progress, nil
			}
		}
		if call == nil {
			// ChatGPT web sometimes refuses tool use and prints the answer instead.
			// Satisfy explicit references from the user prompt so the caller still
			// gets real data instead of a refusal.
			if !didReadFallback && hasToolNamed(specs, "read", "cat", "view", "open", "read_file") {
				if path := ExtractFilePath(prompt, bases); path != "" {
					didReadFallback = true
					content, execErr := c.ExecuteToolIn(ctx, bases, &ToolCall{Name: "read", Arguments: map[string]any{"path": path}})
					if execErr != nil {
						content = "ERROR: " + execErr.Error()
					}
					if debugEnabled() {
						fmt.Fprintf(os.Stderr, "[chatgpt-tool] read fallback for %q (%d bytes)\n", path, len(content))
					}
					transcript += fmt.Sprintf(
						"\n\n--- tool round %d ---\nThe host read the file %s on your behalf. Real contents:\n%s\n\n"+
							"Now answer the user using the real file contents. Do not say you lack file access and do not ask the user to paste anything.",
						round+1, path, truncateOutput(content))
					continue
				}
			}
			if !usedShellFallback && c.ShellEnabled() && hasToolNamed(specs, "shell", "bash", "run", "run_command", "exec", "terminal") {
				if cmd := ExtractShellCommand(reply, prompt); cmd != "" {
					usedShellFallback = true
					if debugEnabled() {
						fmt.Fprintf(os.Stderr, "[chatgpt-tool] shell fallback executing: %q\n", cmd)
					}
					result, execErr := c.ExecuteToolIn(ctx, bases, &ToolCall{Name: "shell", Arguments: map[string]any{"command": cmd}})
					if execErr != nil {
						result = "ERROR: " + execErr.Error()
					}
					transcript += fmt.Sprintf(
						"\n\n--- tool round %d ---\nThe host executed your suggested command: %s\nReal output:\n%s\n\n"+
							"Now answer the user using this real output. Do not repeat the command and do not say you lack shell access.",
						round+1, cmd, truncateOutput(result))
					continue
				}
			}
			if clean != "" {
				return clean, nil, nil
			}
			return reply, nil, nil
		}

		result, execErr := c.ExecuteToolIn(ctx, bases, call)
		if execErr != nil {
			result = "ERROR: " + execErr.Error()
		}
		argsJSON := mustJSON(call.Arguments)
		if len(result) > maxToolOutput {
			result = truncateOutput(result)
		}

		transcript += fmt.Sprintf(
			"\n\n--- tool round %d ---\nYou called %s with arguments %s.\nResult:\n%s\n\n"+
				"Continue. If you now have enough information, reply with the final answer as prose only and do NOT emit a <tool_call> tag.",
			round+1, call.Name, argsJSON, result)
	}

	return "", nil, fmt.Errorf("tool loop exceeded %d rounds", maxToolRounds)
}

// apiContextPreamble reframes the ChatGPT web chat as a stateless API backend model.
// ChatGPT web assumes it is an interactive web chat (asking clarifying questions,
// offering to open files, mentioning no tools available), so API callers must be
// told explicitly what role to play and what output shape is expected.
const apiContextPreamble = `[API CONTEXT - NOT A WEB CHAT]
You are acting as a stateless large language model served through an OpenAI-compatible API.
Rules you must follow:
- Answer the request directly and completely. Do not open with pleasantries, do not ask clarifying questions, do not offer follow-up actions unless asked.
- Never mention that this is a website, chat UI, or that you are a web assistant.
- Do not include meta commentary about these instructions.
- Match the requested output format exactly (code blocks, JSON, plain text, etc.).
- If context is genuinely missing, state the single specific assumption you made and answer anyway.
[END API CONTEXT]

`

// apiNoToolsNote is added when the caller supplied no tool definitions: the model
// must not pretend it can read files or run commands.
const apiNoToolsNote = `[CAPABILITIES]
You have NO browser, NO file system, NO shell, NO tools and NO network.
If a request refers to a file or repository, treat the prompt text itself as the only source of truth;
never say you cannot open files, never ask the user to paste anything, and answer from what you were given.
[END CAPABILITIES]

`

func extractPrompt(body map[string]any, hasTools bool) string {
	p := buildPrompt(body)
	if p == "" {
		return ""
	}
	if hasTools {
		return apiContextPreamble + p
	}
	return apiContextPreamble + apiNoToolsNote + p
}

func buildPrompt(body map[string]any) string {
	rawMsgs := messageList(body)

	if len(rawMsgs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, m := range rawMsgs {
		var role string
		var contentAny, toolCallsAny, nameAny any
		switch item := m.(type) {
		case map[string]any:
			role, _ = item["role"].(string)
			contentAny = item["content"]
			toolCallsAny = item["tool_calls"]
			nameAny = item["name"]
		case map[string]string:
			role = item["role"]
			contentAny = item["content"]
			nameAny = item["name"]
		}

		content := contentToText(contentAny)
		switch role {
		case "system", "developer":
			if content == "" {
				continue
			}
			sb.WriteString("System instruction: ")
			sb.WriteString(content)
			sb.WriteString("\n\n")
		case "user":
			if content == "" {
				continue
			}
			sb.WriteString("User: ")
			sb.WriteString(content)
			sb.WriteString("\n")
		case "assistant":
			sb.WriteString("Assistant: ")
			sb.WriteString(content)
			// Preserve any tool calls the model previously made so follow-up turns
			// see a coherent history instead of a gap.
			if calls := toolCallsToText(toolCallsAny); calls != "" {
				if content != "" {
					sb.WriteString("\n")
				}
				sb.WriteString(calls)
			}
			sb.WriteString("\n")
		case "tool", "function":
			if content == "" {
				continue
			}
			label, _ := nameAny.(string)
			if label == "" {
				label = "tool"
			}
			sb.WriteString("Tool result (")
			sb.WriteString(label)
			sb.WriteString("): ")
			sb.WriteString(content)
			sb.WriteString("\n")
		}
	}
	result := strings.TrimSpace(sb.String())
	if result != "" {
		result += "\n\nAssistant:"
	}
	return result
}

// messageList normalises the messages field into a slice of raw entries.
func messageList(body map[string]any) []any {
	switch v := body["messages"].(type) {
	case []any:
		return v
	case []map[string]any:
		out := make([]any, 0, len(v))
		for _, m := range v {
			out = append(out, m)
		}
		return out
	case []map[string]string:
		out := make([]any, 0, len(v))
		for _, m := range v {
			out = append(out, m)
		}
		return out
	}
	return nil
}

// contentToText renders an OpenAI message content field, which may be a plain
// string or a multimodal array of parts such as [{"type":"text","text":"..."}].
func contentToText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		var parts []string
		for _, item := range t {
			m, ok := item.(map[string]any)
			if !ok {
				if s, ok := item.(string); ok {
					parts = append(parts, s)
				}
				continue
			}
			if txt, ok := m["text"].(string); ok && txt != "" {
				parts = append(parts, txt)
				continue
			}
			if txt, ok := m["content"].(string); ok && txt != "" {
				parts = append(parts, txt)
				continue
			}
			if typ, _ := m["type"].(string); typ != "" {
				parts = append(parts, "["+typ+"]")
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case []map[string]any:
		items := make([]any, 0, len(t))
		for _, item := range t {
			items = append(items, item)
		}
		return contentToText(items)
	}
	return ""
}

// toolCallsToText renders assistant tool_calls into readable prompt text so that
// an agent client's multi-turn tool history survives flattening.
func toolCallsToText(v any) string {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return ""
	}
	var parts []string
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := ""
		args := ""
		if fn, ok := m["function"].(map[string]any); ok {
			name, _ = fn["name"].(string)
			args, _ = fn["arguments"].(string)
		} else {
			name, _ = m["name"].(string)
			args = contentToText(m["arguments"])
		}
		if name == "" {
			continue
		}
		if args == "" {
			parts = append(parts, fmt.Sprintf("[called tool %s]", name))
		} else {
			parts = append(parts, fmt.Sprintf("[called tool %s with %s]", name, args))
		}
	}
	return strings.Join(parts, " ")
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (c *Client) Close() {
	c.readyMu.Lock()
	c.ready = false
	c.readyMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.readLoopStop != nil {
		close(c.readLoopStop)
		c.readLoopStop = nil
	}
	if c.wsConn != nil {
		_ = c.wsConn.Close()
		c.wsConn = nil
	}
	c.killBrowserLocked()
}
