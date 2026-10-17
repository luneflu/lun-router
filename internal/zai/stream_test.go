package zai

import (
	"strings"
	"testing"
	"time"
)

// TestDecoderAppliesEditFrames covers the protocol's real trap: the answer
// arrives as an edit_index/edit_content replacement, not as an append. A parser
// that only reads delta_content returns an empty reply - which is exactly what
// happened during reconnaissance.
func TestDecoderAppliesEditFrames(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"chat:completion","data":{"delta_content":"<details type=\"reasoning\" done=\"false\">\n> The","phase":"thinking"}}`,
		``,
		`data: {"type":"chat:completion","data":{"delta_content":" user wants REUSE_1","phase":"thinking"}}`,
		``,
		`data: {"type":"chat:completion","data":{"edit_index":32,"edit_content":"true\">\n> The user wants REUSE_1\n</details>\nREUSE_1","phase":"answer"}}`,
		``,
		`data: {"type":"chat:completion","data":{"phase":"other","usage":{"prompt_tokens":22473,"completion_tokens":19,"total_tokens":22492}}}`,
		``,
		`data: {"type":"chat:completion","data":{"phase":"done","done":true}}`,
		``,
		`data: [DONE]`,
	}, "\n")

	dec, err := readStream(strings.NewReader(body), nil)
	if err != nil {
		t.Fatalf("readStream: %v", err)
	}

	answer, reasoning := dec.Answer()
	if answer != "REUSE_1" {
		t.Errorf("answer = %q, want %q", answer, "REUSE_1")
	}
	if !strings.Contains(reasoning, "The user wants REUSE_1") {
		t.Errorf("reasoning missing content: %q", reasoning)
	}
	if !strings.Contains(reasoning, `done="true"`) {
		t.Errorf("edit did not rewrite the reasoning tag: %q", reasoning)
	}
	if dec.usage == nil {
		t.Error("usage frame was not captured")
	}
	if !dec.done {
		t.Error("done frame was not captured")
	}
}

// TestDecoderPlainDeltas covers the ordinary shape: reasoning and answer both
// stream as delta_content, as seen in the captured session.
func TestDecoderPlainDeltas(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"chat:completion","data":{"delta_content":"The user is asking","phase":"thinking"}}`,
		`data: {"type":"chat:completion","data":{"delta_content":"Hi! I'm GL","phase":"answer"}}`,
		`data: {"type":"chat:completion","data":{"delta_content":"M-5.3-Flash.","phase":"answer"}}`,
		`data: {"type":"chat:completion","data":{"phase":"done","done":true}}`,
	}, "\n")

	dec, _ := readStream(strings.NewReader(body), nil)
	answer, reasoning := dec.Answer()
	if answer != "Hi! I'm GLM-5.3-Flash." {
		t.Errorf("answer = %q", answer)
	}
	if reasoning != "The user is asking" {
		t.Errorf("reasoning = %q", reasoning)
	}
}

// TestSplitReasoningUnclosedBlock makes sure a truncated reasoning block cannot
// leak into the visible answer.
func TestSplitReasoningUnclosedBlock(t *testing.T) {
	answer, reasoning := SplitReasoning(`<details type="reasoning" done="false">partial thought`)
	if answer != "" {
		t.Errorf("answer should be empty, got %q", answer)
	}
	if !strings.Contains(reasoning, "partial thought") {
		t.Errorf("reasoning = %q", reasoning)
	}
}

// TestSplitReasoningAbsent covers replies with no reasoning block at all.
func TestSplitReasoningAbsent(t *testing.T) {
	answer, reasoning := SplitReasoning("just an answer")
	if answer != "just an answer" {
		t.Errorf("answer = %q", answer)
	}
	if reasoning != "" {
		t.Errorf("reasoning = %q", reasoning)
	}
}

// TestReadStreamSurfacesUpstreamError covers captcha and model-level failures,
// which arrive as a data frame with an error object rather than an HTTP status.
func TestReadStreamSurfacesUpstreamError(t *testing.T) {
	body := `data: {"data":{"done":true,"error":{"code":403,"detail":"Model not available for current user level"}}}` + "\n"
	dec, _ := readStream(strings.NewReader(body), nil)
	if dec.errRaw == nil {
		t.Fatal("error frame was not captured")
	}
	got := upstreamErrorMessage(dec.errRaw)
	if !strings.Contains(got, "Model not available") {
		t.Errorf("message = %q", got)
	}
}

// TestUpstreamErrorNestedShape covers the doubly-nested frame the service uses
// for captcha rejection.
func TestUpstreamErrorNestedShape(t *testing.T) {
	body := `data: {"data":{"data":{"done":true,"error":{"captcha_error_type":"verify_failed","code":"FRONTEND_CAPTCHA_REQUIRED","detail":"Captcha verification failed. Please verify again and retry.","verify_code":"F018"}},"done":true}}` + "\n"
	dec, _ := readStream(strings.NewReader(body), nil)
	if dec.errRaw == nil {
		t.Fatal("nested error frame was not captured")
	}
	got := upstreamErrorMessage(dec.errRaw)
	for _, want := range []string{"FRONTEND_CAPTCHA_REQUIRED", "verify F018", "Captcha verification failed"} {
		if !strings.Contains(got, want) {
			t.Errorf("message %q missing %q", got, want)
		}
	}
	if !isCaptchaError(got) {
		t.Error("captcha error not recognised, so no retry would be attempted")
	}
}

// TestDecodeParamShape verifies a real minted param decodes the way the UI
// expects. The value is a genuine token captured during reconnaissance; it is
// single-use and long since consumed.
func TestDecodeParamShape(t *testing.T) {
	param := "eyJjZXJ0aWZ5SWQiOiJEUmF3b0VFR0h5Iiwic2NlbmVJZCI6ImRpZGszM2UwIiwiaXNTaWduIjp0cnVlLCJzZWN1cml0eVRva2VuIjoiNm9PbzdlNzJuQTYxdVZMaVpWS2lMWXFGMW05ck9ubzN2RUlQSkthTDdLTHhDSnFiMVVCd1JwbDRwN0VjRlRnZGF5MEN3RGJ5YVM0M0U2VTNsZzcveE5qUGNxYWZscWJRTFpRZFgycllkLzhiaG5xaElwQzdTblJsSXhHUHNxdlgifQ=="

	p, err := decodeParam(param)
	if err != nil {
		t.Fatalf("decodeParam: %v", err)
	}
	if p.CertifyID != "DRawoEEGHy" {
		t.Errorf("certifyId = %q", p.CertifyID)
	}
	if p.SceneID != "didk33e0" {
		t.Errorf("sceneId = %q", p.SceneID)
	}
	if !p.IsSign {
		t.Error("isSign should be true")
	}
	if len(p.SecurityToken) != 128 {
		t.Errorf("securityToken length = %d, want 128", len(p.SecurityToken))
	}
	if p.Raw != param {
		t.Error("raw param was not preserved")
	}
}

// TestParamPoolDropsStaleEntries checks that an expired token is never handed
// out, since replaying one always fails upstream.
func TestParamPoolDropsStaleEntries(t *testing.T) {
	var pool paramPool
	pool.put(captchaParam{Raw: "fresh"})

	if _, ok := pool.take(); !ok {
		t.Fatal("fresh param should be handed out")
	}
	if _, ok := pool.take(); ok {
		t.Error("pool should be empty after one take")
	}

	pool.put(captchaParam{Raw: "stale", MintedAt: timeNowMinus(captchaMaxAge * 2)})
	if _, ok := pool.take(); ok {
		t.Error("stale param must not be handed out")
	}
}

// timeNowMinus exists so the pool test can express a stale timestamp without
// importing time at the call site.
func timeNowMinus(d time.Duration) time.Time { return time.Now().Add(-d) }

// TestDecoderTrailingEditIsContinuation pins a live bug. The service streams the
// answer as deltas and then sends a trailing edit whose index is occasionally
// computed from a slightly older snapshot, landing a couple of characters short
// of the end. Applying that as a replacement drops text: a request for
// "ALPHA_BRAVO_CHARLIE" came back as "ALPHA_BVO_CHARLIE" because the edit sat two
// characters before the end of the buffer.
//
// The numbers mirror the captured run: buffer 59 chars, edit index 57.
func TestDecoderTrailingEditIsContinuation(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"chat:completion","data":{"delta_content":"<details type=\"reasoning\" done=\"false\">X","phase":"thinking"}}`,
		`data: {"type":"chat:completion","data":{"edit_index":32,"edit_content":"true\">X</details>\nAL","phase":"answer"}}`,
		`data: {"type":"chat:completion","data":{"delta_content":"PHA_BRA","phase":"answer"}}`,
		`data: {"type":"chat:completion","data":{"phase":"other"}}`,
		`data: {"type":"chat:completion","data":{"edit_index":57,"edit_content":"VO_CHARLIE","phase":"other"}}`,
		`data: {"type":"chat:completion","data":{"phase":"done","done":true}}`,
		`data: [DONE]`,
	}, "\n")

	dec, err := readStream(strings.NewReader(body), nil)
	if err != nil {
		t.Fatalf("readStream: %v", err)
	}
	answer, _ := dec.Answer()
	if answer != "ALPHA_BRAVO_CHARLIE" {
		t.Errorf("answer = %q, want %q (a stale trailing index must append, not truncate)", answer, "ALPHA_BRAVO_CHARLIE")
	}
}

// TestDecoderMidBufferEditStillReplaces guards the other side of the tolerance.
// An edit replaces from its index to the end of the buffer, so its content
// carries the remainder; a genuine revision far from the end must still
// overwrite rather than append.
func TestDecoderMidBufferEditStillReplaces(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"chat:completion","data":{"delta_content":"AAAAxxxxBBBB","phase":"thinking"}}`,
		`data: {"type":"chat:completion","data":{"edit_index":4,"edit_content":"yyyyBBBB","phase":"answer"}}`,
		`data: {"type":"chat:completion","data":{"phase":"done","done":true}}`,
	}, "\n")

	dec, _ := readStream(strings.NewReader(body), nil)
	// Assert the buffer itself: Answer() reports only the answer-phase slice, and
	// what is under test here is that the edit overwrote the middle rather than
	// being appended.
	if dec.buf != "AAAAyyyyBBBB" {
		t.Errorf("buffer = %q, want %q", dec.buf, "AAAAyyyyBBBB")
	}
}
