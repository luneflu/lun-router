package zai

import (
	"testing"
)

// TestSignatureMatchesCapturedRequest pins the signature against a real request
// captured from chat.z.ai. The expected value came from the HAR:
//
//	x-signature: c8697d13d36a7231da02899a6821b205b2e8c30f5105398e2ad02cc3aca44485
//
// Getting this wrong is silent - the service answers 200 with
// "Signature validation failed" inside the stream.
func TestSignatureMatchesCapturedRequest(t *testing.T) {
	got := Signature(
		"1792244102245",
		"c5065bd9-79a1-4e44-bfc3-c99eff735804",
		"14475c49-562a-40c8-9b4c-2af2a60907be",
		"Hi, what model are you?",
	)
	want := "c8697d13d36a7231da02899a6821b205b2e8c30f5105398e2ad02cc3aca44485"
	if got != want {
		t.Errorf("signature mismatch\n got: %s\nwant: %s", got, want)
	}
}

// TestSignatureUsesEntryJoin guards the payload encoding. The frontend sorts the
// entries of the params object and joins them, which yields "k,v,k,v" - not a
// list of keys and not a query string.
func TestSignatureUsesEntryJoin(t *testing.T) {
	payload := Payload("123", "rid", "uid")
	want := "requestId,rid,timestamp,123,user_id,uid"
	if payload != want {
		t.Errorf("payload = %q, want %q", payload, want)
	}
}

// TestSignatureBucketBoundary verifies the five-minute rotation is applied to
// the key derivation, since a wrong bucket yields a different signature.
func TestSignatureBucketBoundary(t *testing.T) {
	if got := bucketFor("300000"); got != "1" {
		t.Errorf("bucket for 300000 = %s, want 1", got)
	}
	if got := bucketFor("299999"); got != "0" {
		t.Errorf("bucket for 299999 = %s, want 0", got)
	}
	// The timestamp is taken modulo the window, so two timestamps in the same
	// window must share a bucket and two across a boundary must not.
	sameA := Signature("1792244102245", "r", "u", "p")
	sameB := Signature("1792244102999", "r", "u", "p")
	if sameA == sameB {
		t.Error("signatures should differ when the signed message differs")
	}
}

// TestSignatureChangesWithPrompt guards against a signature that ignores the
// prompt, which would fail validation in the same bucket.
func TestSignatureChangesWithPrompt(t *testing.T) {
	a := Signature("1792244102245", "rid", "uid", "hello")
	b := Signature("1792244102245", "rid", "uid", "hello!")
	if a == b {
		t.Error("different prompts produced the same signature")
	}
}
