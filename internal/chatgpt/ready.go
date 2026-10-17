package chatgpt

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// waitForDocument blocks until the ChatGPT document reports readyState complete
// and the expected origin, then waits for the app to become interactive.
// Cold starts on a slow machine can take many seconds; acting before this point
// silently breaks SDK injection or races the SPA's own startup fetch.
func (c *Client) waitForDocument(ctx context.Context) error {
	expr := `new Promise((res) => {
		const interactive = () => document.readyState === 'complete'
			&& location.host === 'chatgpt.com'
			&& !!document.querySelector('#mobile-composer-prompt, div[contenteditable="true"], textarea');
		if (interactive()) return res('ready');
		const started = Date.now();
		const t = setInterval(() => {
			if (interactive()) {
				clearInterval(t); res('ready');
			} else if (Date.now() - started > 12000) {
				clearInterval(t);
				res('wait:' + document.readyState + ':' + location.href.slice(0, 80));
			}
		}, 200);
	})`

	deadline := time.Now().Add(75 * time.Second)
	var last string
	for attempt := 0; attempt < 6; attempt++ {
		if time.Now().After(deadline) {
			break
		}
		val, err := c.evaluateString(ctx, expr)
		if err == nil && val == "ready" {
			// Give the SPA a moment to finish its own startup requests so our
			// first turn does not race a client-side redirect.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1500 * time.Millisecond):
			}
			return nil
		}
		last = fmt.Sprintf("err=%v val=%q", err, val)
		if debugEnabled() {
			fmt.Fprintf(os.Stderr, "[chatgpt] waitForDocument attempt %d: %s\n", attempt+1, last)
		}

		// The document may be sitting on about:blank or an error page; drive the
		// navigation again rather than waiting for a page that will never load.
		if err == nil && !strings.HasPrefix(val, "ready") {
			_ = c.sendCDP("Page.navigate", map[string]any{"url": "https://chatgpt.com/"})
		}
	}
	return fmt.Errorf("chatgpt.com never became interactive (%s)", last)
}

// ensureSentinelReady injects the sentinel SDK if absent and verifies its
// methods are actually callable. A half-initialised SDK object (present but with
// no callable methods) cannot be repaired in place, so the page is reloaded to
// obtain a clean document rather than polling until the deadline.
func (c *Client) ensureSentinelReady(ctx context.Context) error {
	probe := `(function(){
		if (window.SentinelSDK && typeof window.SentinelSDK.token === 'function'
			&& typeof window.SentinelSDK.requirementsToken === 'function') return 'ready';
		return window.SentinelSDK ? 'partial' : 'missing';
	})()`

	inject := `(function(){
		if (window.SentinelSDK) return 'present';
		try {
			const s = document.createElement('script');
			s.src = '/backend-api/sentinel/sdk.js';
			s.async = true;
			document.head.appendChild(s);
			return 'injected';
		} catch (e) { return 'error:' + e; }
	})()`

	deadline := time.Now().Add(75 * time.Second)
	var last string
	for attempt := 0; attempt < 6; attempt++ {
		if time.Now().After(deadline) {
			break
		}

		val, err := c.evaluateStringOnPageCtx(ctx, probe)
		if err == nil && val == "ready" {
			if debugEnabled() {
				fmt.Fprintf(os.Stderr, "[chatgpt] sentinel ready after %d attempt(s)\n", attempt+1)
			}
			return nil
		}
		last = fmt.Sprintf("err=%v state=%q", err, val)
		if debugEnabled() {
			fmt.Fprintf(os.Stderr, "[chatgpt] ensureSentinel attempt %d: %s\n", attempt+1, last)
		}

		if err == nil && val == "missing" {
			if _, ierr := c.evaluateStringOnPageCtx(ctx, inject); ierr != nil && debugEnabled() {
				fmt.Fprintf(os.Stderr, "[chatgpt] sentinel injection error: %v\n", ierr)
			}
		} else if attempt >= 1 && err == nil && val == "partial" {
			// Object exists but is unusable: reload for a clean document.
			if debugEnabled() {
				fmt.Fprintln(os.Stderr, "[chatgpt] sentinel SDK partial; reloading page")
			}
			_ = c.sendCDP("Page.navigate", map[string]any{"url": "https://chatgpt.com/"})
			if werr := c.waitForDocument(ctx); werr != nil {
				return werr
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1200 * time.Millisecond):
		}
	}
	return fmt.Errorf("sentinel SDK never became callable (%s)", last)
}

// evaluateString runs an expression awaiting promises and returns its string
// result. NotImplemented CDP responses surface as an error.
func (c *Client) evaluateString(ctx context.Context, expression string) (string, error) {
	return c.evaluateStringOnPage(expression)
}
