package zai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// browser is a minimal CDP driver: just enough to load one page and evaluate
// expressions on it. The z.ai flow needs a browser for exactly one thing - the
// Aliyun captcha - so there is no DOM driving, no input synthesis and no
// in-page request protocol here.
type browser struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	conn     *websocket.Conn
	writeMu  sync.Mutex
	pending  map[int64]chan cdpReply
	callID   atomic.Int64
	port     string
	profile  string
	pageWS   string
	closed   bool
	navReady bool
}

type cdpReply struct {
	result json.RawMessage
	err    error
}

// launchBrowser starts Chromium with a debug port and attaches to a fresh page.
func launchBrowser(ctx context.Context, executable, port, profileDir string) (*browser, error) {
	if executable == "" {
		return nil, errors.New("no browser executable found")
	}
	if port == "" {
		port = "9333"
	}
	if profileDir == "" {
		profileDir = filepath.Join(os.TempDir(), "lun_zai_profile")
	}
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return nil, fmt.Errorf("create profile dir: %w", err)
	}

	args := []string{
		"--headless=new",
		fmt.Sprintf("--remote-debugging-port=%s", port),
		"--remote-allow-origins=*",
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		"--window-size=1440,900",
		"--lang=en-US",
		"--mute-audio",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-breakpad",
		"--disable-component-update",
		"--disable-extensions",
		"--disable-sync",
		"--disable-features=Translate,OptimizationHints,MediaRouter,CertificateTransparencyComponentUpdater",
		"--metrics-recording-only",
		"about:blank",
	}

	cmd := exec.Command(executable, args...)
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("exec %s: %w", executable, err)
	}

	b := &browser{
		cmd:     cmd,
		port:    port,
		profile: profileDir,
		pending: make(map[int64]chan cdpReply),
	}

	// Wait for the debug endpoint, then open a page target we can drive.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if b.pageWS == "" {
			if ws := b.openPageTarget(ctx); ws != "" {
				b.pageWS = ws
			}
		}
		if b.pageWS != "" {
			if err := b.dial(ctx); err == nil {
				return b, nil
			}
			b.pageWS = ""
		}
		select {
		case <-ctx.Done():
			b.Close()
			return nil, ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	b.Close()
	return nil, errors.New("browser debug endpoint never became usable")
}

// openPageTarget asks the browser for a fresh page target and returns its
// websocket URL. Chrome requires PUT for /json/new.
func (b *browser) openPageTarget(ctx context.Context) string {
	endpoint := fmt.Sprintf("http://127.0.0.1:%s/json/new?url=about:blank", b.port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var t struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if json.NewDecoder(resp.Body).Decode(&t) != nil {
		return ""
	}
	return t.WebSocketDebuggerURL
}

// pid reports the root process id of the launched browser, or 0 when it is not
// running.
func (b *browser) pid() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd == nil || b.cmd.Process == nil {
		return 0
	}
	return b.cmd.Process.Pid
}

func (b *browser) dial(ctx context.Context) error {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, b.pageWS, nil)
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.conn = conn
	b.mu.Unlock()
	go b.readLoop(conn)
	return nil
}

func (b *browser) readLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			b.mu.Lock()
			if b.conn == conn {
				b.conn = nil
			}
			b.mu.Unlock()
			return
		}
		var msg struct {
			ID     *int64          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &msg) != nil || msg.ID == nil {
			continue // CDP events are not correlated to a call
		}
		b.mu.Lock()
		ch := b.pending[*msg.ID]
		delete(b.pending, *msg.ID)
		b.mu.Unlock()
		if ch == nil {
			continue
		}
		reply := cdpReply{result: msg.Result}
		if len(msg.Error) > 0 && string(msg.Error) != "null" {
			reply.err = fmt.Errorf("%s", strings.TrimSpace(string(msg.Error)))
		}
		ch <- reply
	}
}

// call sends one CDP command and waits for its correlated reply.
func (b *browser) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	b.mu.Lock()
	conn := b.conn
	closed := b.closed
	b.mu.Unlock()
	if conn == nil || closed {
		return nil, errors.New("cdp not connected")
	}

	id := b.callID.Add(1)
	ch := make(chan cdpReply, 1)
	b.mu.Lock()
	b.pending[id] = ch
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
	}()

	payload, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	b.writeMu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, payload)
	b.writeMu.Unlock()
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case reply := <-ch:
		return reply.result, reply.err
	}
}

// evaluate runs an expression on the page and returns its string value, with
// awaitPromise so it can wait on the captcha promise.
func (b *browser) evaluate(ctx context.Context, expression string, await bool) (string, error) {
	raw, err := b.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"awaitPromise":  await,
		"returnByValue": true,
	})
	if err != nil {
		return "", err
	}
	var res struct {
		Result struct {
			Value any `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception *struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.ExceptionDetails != nil {
		desc := res.ExceptionDetails.Text
		if res.ExceptionDetails.Exception != nil && res.ExceptionDetails.Exception.Description != "" {
			desc = res.ExceptionDetails.Exception.Description
		}
		return "", fmt.Errorf("page exception: %s", strings.TrimSpace(desc))
	}
	if res.Result.Value == nil {
		return "", nil
	}
	if s, ok := res.Result.Value.(string); ok {
		return s, nil
	}
	buf, _ := json.Marshal(res.Result.Value)
	return string(buf), nil
}

// navigate loads url and waits until the document reports complete.
func (b *browser) navigate(ctx context.Context, url string) error {
	if _, err := b.call(ctx, "Page.enable", map[string]any{}); err != nil {
		return err
	}
	if _, err := b.call(ctx, "Page.navigate", map[string]any{"url": url}); err != nil {
		return err
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		state, err := b.evaluate(ctx, "document.readyState + '|' + location.host", false)
		if err != nil {
			continue
		}
		if strings.HasPrefix(state, "complete|") && strings.Contains(state, "chat.z.ai") {
			return nil
		}
	}
	return errors.New("page never finished loading")
}

// Close tears down the connection and the whole browser process group. Orphaned
// Chromium helpers keep the debug port bound and poison the next launch.
func (b *browser) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	conn := b.conn
	b.conn = nil
	cmd := b.cmd
	profile := b.profile
	b.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	if cmd != nil && cmd.Process != nil {
		killProcessGroup(cmd.Process.Pid)
		_, _ = cmd.Process.Wait()
	}
	if profile != "" {
		_ = os.RemoveAll(profile)
	}
}
